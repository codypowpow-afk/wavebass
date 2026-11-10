// Package ui is the WaveBass front panel. It draws through the small Canvas interface and
// takes plain mouse / key / MIDI events, so it has no dependency on the windowing library.
package ui

import (
	"fmt"
	"image/color"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codypowpow-afk/wavebass/synth"
)

// Logical window size.
const (
	W = 1440
	H = 860
)

type Font int

const (
	FTiny Font = iota
	FSmall
	FBody
	FBold
	FBig
	FTitle
)

const (
	AlignL = iota
	AlignC
	AlignR
)

// Canvas is everything the UI needs to draw. Text y is the top of the line.
type Canvas interface {
	Rect(x, y, w, h float32, c color.RGBA)
	Circle(x, y, r float32, c color.RGBA)
	Arc(cx, cy, r, a0, a1, width float32, c color.RGBA)
	Polyline(pts []float32, width float32, c color.RGBA)
	Text(s string, f Font, x, y float32, c color.RGBA, align int)
	// Layer makes everything drawn afterwards appear above everything drawn before.
	Layer()
}

var (
	cBG      = color.RGBA{0x11, 0x12, 0x15, 0xff}
	cPanel   = color.RGBA{0x1b, 0x1d, 0x23, 0xff}
	cPanelHi = color.RGBA{0x2a, 0x2d, 0x36, 0xff}
	cWell    = color.RGBA{0x0c, 0x0d, 0x10, 0xff}
	cLine    = color.RGBA{0x3a, 0x3e, 0x49, 0xff}
	cText    = color.RGBA{0xec, 0xe9, 0xe2, 0xff}
	cDim     = color.RGBA{0x8d, 0x92, 0x9d, 0xff}
	cAccent  = color.RGBA{0xff, 0x7a, 0x1a, 0xff}
	cAccent2 = color.RGBA{0x35, 0xd0, 0xc0, 0xff}
	cRed     = color.RGBA{0xff, 0x4d, 0x4d, 0xff}
	cGreen   = color.RGBA{0x5f, 0xd3, 0x8a, 0xff}
	cYellow  = color.RGBA{0xf2, 0xc9, 0x4c, 0xff}
)

var catColor = map[string]color.RGBA{
	"Sub":          {0x5b, 0x8d, 0xef, 0xff},
	"Classic":      {0xf2, 0xc9, 0x4c, 0xff},
	"Acid":         {0xb6, 0xe0, 0x3c, 0xff},
	"FM / Digital": {0x35, 0xd0, 0xc0, 0xff},
	"Wobble":       {0xff, 0x7a, 0x1a, 0xff},
	"Growl":        {0xff, 0x4d, 0x4d, 0xff},
	"Neuro / DnB":  {0xc7, 0x7d, 0xff, 0xff},
	"Weird":        {0xff, 0x8f, 0xc8, 0xff},
	"Poly / Stab":  {0x9a, 0xa7, 0xb8, 0xff},
	"User":         {0xec, 0xe9, 0xe2, 0xff},
}

const (
	wKnob = iota
	wStep
	wInline
	wToggle
	wSlider
)

type widget struct {
	kind       int
	pid        synth.PID
	x, y, w, h float32
}

type panel struct {
	title      string
	x, y, w, h float32
}

type button struct {
	x, y, w, h float32
	label      func(a *App) string
	on         func(a *App) bool
	act        func(a *App, shift bool)
	col        color.RGBA
	big        bool
}

// Layout.
const (
	browX, browY, browW = 12, 62, 224
	browRowH            = 21
	browListY           = browY + 36
	browRows            = 35
	mainX               = 248
	mainW               = W - 12 - mainX
	rowOscY             = 62
	rowOscH             = 118
	row2Y               = 188
	row3Y               = 288
	row4Y               = 388
	rowH                = 92
	matY                = 488
	matH                = 104
	hwY                 = 600
	hwH                 = 118
	loopY               = 726
	loopH               = 40
	kbX                 = mainX + 2
	kbY                 = 774
	kbH                 = 74
	kbLow               = 12
	kbHigh              = 95
	kbWhiteW            = 24
	scopeW              = 136
)

// UserPreset is a saved sound in the config file.
type UserPreset struct {
	Name   string             `json:"name"`
	Values map[string]float64 `json:"values"`
	Knobs  []string           `json:"knobs"`
}

// Config is what is remembered between runs.
type Config struct {
	KnobCC     [8]int             `json:"knob_cc"`
	KnobLock   bool               `json:"knob_lock"`
	LockKnobs  []string           `json:"lock_knobs,omitempty"`
	MIDIOctave int                `json:"midi_octave"`
	KbBase     int                `json:"keyboard_base"`
	BPM        float64            `json:"bpm"`
	Last       string             `json:"last_preset"`
	Globals    map[string]float64 `json:"performance"`
	User       []UserPreset       `json:"user_presets"`
}

// DefaultConfig is used on first run.
func DefaultConfig() Config {
	return Config{KnobCC: [8]int{-1, -1, -1, -1, -1, -1, -1, -1}, MIDIOctave: -2, KbBase: 24, BPM: 120}
}

// App is the whole front panel state.
type App struct {
	Eng *synth.Engine
	st  synth.Status

	Presets []synth.Preset
	cur     int
	cat     int // -1 = all
	scroll  int
	list    []int // preset indices shown in the browser

	mu       sync.Mutex // guards the knob mapping (also touched by the MIDI thread)
	knobs    [8]synth.PID
	knobCC   [8]int
	knobLock bool
	learn    int // slot waiting for a CC (-1 none)
	pickSlot int // slot waiting for a control to be clicked (-1 none)
	pickPID  synth.PID
	pickOn   bool // a control was right-clicked: waiting for a hardware knob
	knobT    [8]time.Time
	lastCC   int
	lastCCv  int

	midiOct     atomic.Int32
	playing     [128]atomic.Int32
	midiNotes   atomic.Int64
	midiSeen    int64
	midiFlash   time.Time
	pendingProg atomic.Int32
	Devices     func() (open, busy []string)
	Rescan      func()
	OpenFolder  func(dir string)
	RecDir      string
	SaveConfig  func(c *Config)
	dirty       bool
	dirtyT      time.Time

	kbBase    int
	mouseNote int

	panels  []panel
	widgets []widget
	buttons []button
	drag    *widget
	dragHW  int
	dragBPM bool
	dragY   float32
	dragN   float64
	lastClk time.Time
	lastW   *widget

	naming  bool
	nameBuf string

	taps    []time.Time
	toast   string
	toastT  time.Time
	extOn   bool
	lastRec string
}

// New builds the panel. cfg restores the previous session.
func New(eng *synth.Engine, cfg Config) *App {
	a := &App{Eng: eng, cat: -1, learn: -1, pickSlot: -1, dragHW: -1, mouseNote: -1, extOn: true, lastCC: -1}
	a.Presets = append(a.Presets, synth.Presets...)
	for _, u := range cfg.User {
		a.Presets = append(a.Presets, fromUser(u))
	}
	a.knobCC = cfg.KnobCC
	for i, cc := range a.knobCC {
		if cc < 0 || cc > 127 {
			a.knobCC[i] = -1
		}
	}
	a.knobLock = cfg.KnobLock
	a.midiOct.Store(int32(clampI(cfg.MIDIOctave, -4, 4)))
	a.kbBase = clampI(cfg.KbBase, 0, 96)
	if cfg.BPM >= 30 {
		eng.SetBPM(cfg.BPM)
	}
	for k, v := range cfg.Globals {
		if id, ok := synth.KeyToPID(k); ok && synth.Metas[id].Global && id != synth.PArpMode {
			eng.Set(id, v)
		}
	}
	a.knobs = synth.KnobsStd
	a.layout()
	sel := 0
	for i := range a.Presets {
		if a.Presets[i].Name == cfg.Last {
			sel = i
		}
	}
	a.selectPreset(sel, false)
	if a.knobLock && len(cfg.LockKnobs) == 8 {
		for i, k := range cfg.LockKnobs {
			if id, ok := synth.KeyToPID(k); ok {
				a.knobs[i] = id
			}
		}
	}
	a.rebuildList()
	a.st = eng.Snapshot()
	a.dirty = false
	return a
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func fromUser(u UserPreset) synth.Preset {
	p := synth.Preset{Name: u.Name, Cat: "User", Patch: synth.DefaultPatch(), Knobs: synth.KnobsStd, User: true}
	for k, v := range u.Values {
		if id, ok := synth.KeyToPID(k); ok {
			p.Patch[id] = synth.Metas[id].Clamp(v)
		}
	}
	for i, k := range u.Knobs {
		if id, ok := synth.KeyToPID(k); ok && i < 8 {
			p.Knobs[i] = id
		}
	}
	return p
}

func toUser(p *synth.Preset) UserPreset {
	u := UserPreset{Name: p.Name, Values: map[string]float64{}}
	for i := range synth.Metas {
		if !synth.Metas[i].Global {
			u.Values[synth.Metas[i].Key] = p.Patch[i]
		}
	}
	for _, k := range p.Knobs {
		u.Knobs = append(u.Knobs, synth.Metas[k].Key)
	}
	return u
}

func (a *App) config() Config {
	c := DefaultConfig()
	a.mu.Lock()
	c.KnobCC = a.knobCC
	c.KnobLock = a.knobLock
	for _, k := range a.knobs {
		c.LockKnobs = append(c.LockKnobs, synth.Metas[k].Key)
	}
	a.mu.Unlock()
	c.MIDIOctave = int(a.midiOct.Load())
	c.KbBase = a.kbBase
	c.BPM = math.Round(a.st.BPM*10) / 10
	c.Last = a.Presets[a.cur].Name
	c.Globals = map[string]float64{}
	for i := range synth.Metas {
		if synth.Metas[i].Global {
			c.Globals[synth.Metas[i].Key] = a.st.P[i]
		}
	}
	for i := range a.Presets {
		if a.Presets[i].User {
			c.User = append(c.User, toUser(&a.Presets[i]))
		}
	}
	return c
}

func (a *App) markDirty() {
	if !a.dirty {
		a.dirty, a.dirtyT = true, time.Now()
	}
}

// Flush writes the settings now (call on exit).
func (a *App) Flush() {
	if a.SaveConfig != nil {
		c := a.config()
		a.SaveConfig(&c)
	}
	a.dirty = false
}

func (a *App) say(s string) { a.toast, a.toastT = s, time.Now() }

// ---------------------------------------------------------------- layout

func (a *App) cells(title string, x, y, w, h float32, cw float32, pids ...synth.PID) {
	a.panels = append(a.panels, panel{title, x, y, w, h})
	for i, id := range pids {
		a.addCell(id, x+8+float32(i)*cw, y+22, cw)
	}
}

func (a *App) addCell(id synth.PID, x, y, cw float32) {
	m := &synth.Metas[id]
	k := wKnob
	if m.Stepped() {
		k = wStep
		if m.Kind == synth.KChoice && len(m.Choices) == 2 {
			k = wToggle
		}
	}
	a.widgets = append(a.widgets, widget{k, id, x, y, cw, 62})
}

func (a *App) layout() {
	// Oscillators.
	oscW := float32(mainW-8) / 2
	for o := 0; o < 2; o++ {
		base := synth.PATable + synth.PID(9*o)
		x := float32(mainX) + float32(o)*(oscW+8)
		a.panels = append(a.panels, panel{[]string{"OSC A", "OSC B"}[o], x, rowOscY, oscW, rowOscH})
		a.widgets = append(a.widgets, widget{wInline, base, x + 8, rowOscY + rowOscH - 30, scopeW, 22})
		cw := (oscW - 24 - scopeW) / 8
		for i := 1; i <= 8; i++ {
			a.addCell(base+synth.PID(i), x+16+scopeW+float32(i-1)*cw, rowOscY+36, cw)
		}
	}
	row := func(y float32, groups ...[]synth.PID) {
		n := 0
		for _, g := range groups {
			n += len(g)
		}
		cw := float32(math.Floor(float64((float32(mainW) - float32(len(groups)-1)*8 - float32(len(groups))*16) / float32(n))))
		x := float32(mainX)
		for _, g := range groups {
			w := cw*float32(len(g)) + 16
			a.cells(synth.Metas[g[0]].Sec, x, y, w, rowH, cw, g...)
			x += w + 8
		}
		// stretch the last panel to the right edge
		p := &a.panels[len(a.panels)-1]
		p.w = float32(mainX+mainW) - p.x
	}
	seq := func(from, to synth.PID) []synth.PID {
		var s []synth.PID
		for i := from; i <= to; i++ {
			s = append(s, i)
		}
		return s
	}
	row(row2Y, seq(synth.PSubShape, synth.PNoise), seq(synth.PFType, synth.PFKey), seq(synth.PMode, synth.PVelSens), seq(synth.PBass, synth.PVolume))
	// FM lives with the sub / noise group.
	a.panels[len(a.panels)-4].title = "SUB / NOISE"
	row(row3Y, seq(synth.PAmpA, synth.PAmpR), seq(synth.PFltA, synth.PFltR), seq(synth.PModA, synth.PModR), seq(synth.PL1Shape, synth.PL1Retrig), seq(synth.PL2Shape, synth.PL2Retrig))
	row(row4Y, append([]synth.PID{synth.PFM}, seq(synth.PDistType, synth.PRevMix)...), seq(synth.PArpMode, synth.PArpLatch))
	a.panels[len(a.panels)-2].title = "FM  /  EFFECTS"

	// Mod matrix.
	a.panels = append(a.panels, panel{"MOD MATRIX   source > destination, amount", mainX, matY, mainW, matH})
	sw := (float32(mainW) - 16 - 5*8) / 6
	for s := 0; s < synth.ModSlots; s++ {
		b := synth.PM1Src + synth.PID(3*s)
		x := float32(mainX) + 8 + float32(s)*(sw+8)
		a.widgets = append(a.widgets,
			widget{wInline, b, x, matY + 22, sw/2 - 2, 24},
			widget{wInline, b + 1, x + sw/2 + 2, matY + 22, sw/2 - 2, 24},
			widget{wSlider, b + 2, x, matY + 54, sw, 38})
	}
	a.panels = append(a.panels, panel{"YOUR 8 KEYBOARD KNOBS", mainX, hwY, mainW, hwH})
	a.panels = append(a.panels, panel{"", mainX, loopY, mainW, loopH})
	a.panels = append(a.panels, panel{"", browX, browY, browW, H - 12 - browY})

	a.makeButtons()
}

func (a *App) btn(x, y, w, h float32, label func(*App) string, on func(*App) bool, act func(*App, bool)) *button {
	a.buttons = append(a.buttons, button{x: x, y: y, w: w, h: h, label: label, on: on, act: act, col: cAccent})
	return &a.buttons[len(a.buttons)-1]
}

func lbl(s string) func(*App) string { return func(*App) string { return s } }

func (a *App) makeButtons() {
	never := func(*App) bool { return false }
	// Preset navigation.
	a.btn(196, 12, 30, 34, lbl("<"), never, func(a *App, _ bool) { a.stepPreset(-1) })
	a.btn(560, 12, 30, 34, lbl(">"), never, func(a *App, _ bool) { a.stepPreset(1) })
	a.btn(598, 12, 56, 34, lbl("SAVE"), func(a *App) bool { return a.naming }, func(a *App, _ bool) { a.startNaming() })
	a.btn(658, 12, 44, 34, lbl("DEL"), never, func(a *App, _ bool) { a.deleteUser() })
	// Tempo.
	a.btn(722, 12, 26, 34, lbl("-"), never, func(a *App, sh bool) { a.nudgeBPM(-1, sh) })
	a.btn(826, 12, 26, 34, lbl("+"), never, func(a *App, sh bool) { a.nudgeBPM(1, sh) })
	a.btn(856, 12, 46, 34, lbl("TAP"), never, func(a *App, _ bool) { a.tap() })
	a.btn(906, 12, 62, 34, lbl("CLICK"), func(a *App) bool { return a.st.Metro }, func(a *App, _ bool) { a.Eng.SetMetronome(!a.st.Metro) })
	a.btn(972, 12, 74, 34, func(a *App) string {
		if a.st.Ext {
			return "EXT SYNC"
		}
		if a.extOn {
			return "CLK AUTO"
		}
		return "CLK OFF"
	}, func(a *App) bool { return a.st.Ext }, func(a *App, _ bool) {
		a.extOn = !a.extOn
		a.Eng.SetExtSync(a.extOn)
		if a.extOn {
			a.say("Tempo follows MIDI clock when your keyboard / DAW sends it")
		} else {
			a.say("MIDI clock ignored")
		}
	}).col = cAccent2
	// Recording.
	r := a.btn(1058, 12, 106, 34, func(a *App) string {
		if a.st.Recording {
			return fmt.Sprintf("STOP  %d:%02d", int(a.st.RecSeconds)/60, int(a.st.RecSeconds)%60)
		}
		return "REC WAV"
	}, func(a *App) bool { return a.st.Recording }, func(a *App, _ bool) { a.toggleRec() })
	r.col = cRed
	a.btn(1168, 12, 30, 34, lbl("..."), never, func(a *App, _ bool) {
		if a.OpenFolder != nil && a.RecDir != "" {
			a.OpenFolder(a.RecDir)
		}
	})
	// MIDI.
	a.btn(1338, 30, 20, 18, lbl("<"), never, func(a *App, _ bool) { a.shiftMIDIOct(-1) })
	a.btn(1408, 30, 20, 18, lbl(">"), never, func(a *App, _ bool) { a.shiftMIDIOct(1) })
	// Browser.
	a.btn(browX+6, browY+6, 24, 24, lbl("<"), never, func(a *App, _ bool) { a.stepCat(-1) })
	a.btn(browX+browW-30, browY+6, 24, 24, lbl(">"), never, func(a *App, _ bool) { a.stepCat(1) })
	// Hardware knob options.
	a.btn(mainX+mainW-268, hwY+3, 126, 16, func(a *App) string {
		if a.knobLock {
			return "KNOBS: LOCKED"
		}
		return "KNOBS: PER PRESET"
	}, func(a *App) bool { return a.knobLock }, func(a *App, _ bool) {
		a.mu.Lock()
		a.knobLock = !a.knobLock
		a.mu.Unlock()
		if a.knobLock {
			a.say("Knob assignments now stay the same for every preset")
		} else {
			a.say("Each preset sets up the 8 knobs for itself")
		}
		a.markDirty()
	})
	a.btn(mainX+mainW-136, hwY+3, 128, 16, lbl("RE-LEARN ALL KNOBS"), never, func(a *App, _ bool) {
		a.mu.Lock()
		for i := range a.knobCC {
			a.knobCC[i] = -1
		}
		a.learn = -1
		a.mu.Unlock()
		a.say("Knob map cleared: now turn your 8 knobs from left to right")
		a.markDirty()
	})
	// Loop recorder.
	x := float32(mainX + 8)
	lr := a.btn(x, loopY+6, 96, 28, func(a *App) string {
		switch a.st.Loop.State {
		case 1:
			return "ARMED: PLAY"
		case 2:
			return "RECORDING"
		case 3:
			if a.st.Loop.Overdub {
				return "OVERDUB ON"
			}
			return "OVERDUB"
		}
		return "LOOP REC"
	}, func(a *App) bool {
		l := a.st.Loop
		return l.State == 2 || (l.State == 3 && l.Overdub) || (l.State == 1 && time.Now().UnixMilli()/300%2 == 0)
	}, func(a *App, _ bool) {
		a.Eng.LoopRec()
		if a.Eng.Snapshot().Loop.State == 1 {
			a.say("Loop armed: recording starts with the first note you play")
		}
	})
	lr.col = cRed
	a.btn(x+100, loopY+6, 64, 28, func(a *App) string {
		if a.st.Loop.State == 3 || a.st.Loop.State == 2 {
			return "STOP"
		}
		return "PLAY"
	}, func(a *App) bool { return a.st.Loop.State == 3 }, func(a *App, _ bool) { a.Eng.LoopPlay() }).col = cGreen
	a.btn(x+168, loopY+6, 60, 28, lbl("CLEAR"), never, func(a *App, _ bool) { a.Eng.LoopClear() })
	a.btn(x+232, loopY+6, 70, 28, func(a *App) string {
		if a.st.Loop.Bars == 1 {
			return "1 BAR"
		}
		return fmt.Sprintf("%d BARS", a.st.Loop.Bars)
	}, never, func(a *App, _ bool) {
		if a.st.Loop.State > 1 {
			a.say("Clear the loop to change its length")
			return
		}
		next := map[int]int{1: 2, 2: 4, 4: 8, 8: 1}[a.st.Loop.Bars]
		if next == 0 {
			next = 2
		}
		a.Eng.SetLoopBars(next)
	})
	a.btn(x+306, loopY+6, 84, 28, lbl("QUANT 1/16"), func(a *App) bool { return a.st.Loop.Quant }, func(a *App, _ bool) { a.Eng.SetLoopQuant(!a.st.Loop.Quant) }).col = cAccent2
	a.btn(mainX+mainW-88, loopY+6, 80, 28, lbl("ALL OFF"), never, func(a *App, _ bool) { a.Panic() })
}

// ---------------------------------------------------------------- presets

func (a *App) rebuildList() {
	a.list = a.list[:0]
	for i := range a.Presets {
		if a.cat < 0 || a.Presets[i].Cat == synth.Categories[a.cat] {
			a.list = append(a.list, i)
		}
	}
	a.scroll = clampI(a.scroll, 0, maxI(0, len(a.list)-browRows))
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) stepCat(d int) {
	n := len(synth.Categories)
	a.cat = (a.cat+1+d+(n+1))%(n+1) - 1
	a.scroll = 0
	a.rebuildList()
}

func (a *App) selectPreset(i int, announce bool) {
	if i < 0 || i >= len(a.Presets) {
		return
	}
	a.cur = i
	p := &a.Presets[i]
	a.Eng.LoadPatch(&p.Patch)
	a.mu.Lock()
	if !a.knobLock {
		a.knobs = p.Knobs
	}
	a.mu.Unlock()
	if announce {
		a.say(p.Name)
	}
	// keep it visible in the browser
	for row, idx := range a.list {
		if idx == i {
			if row < a.scroll {
				a.scroll = row
			}
			if row >= a.scroll+browRows {
				a.scroll = row - browRows + 1
			}
		}
	}
	a.markDirty()
}

func (a *App) stepPreset(d int) {
	if len(a.list) == 0 {
		return
	}
	pos := -1
	for row, idx := range a.list {
		if idx == a.cur {
			pos = row
		}
	}
	if pos < 0 {
		a.selectPreset(a.list[0], true)
		return
	}
	a.selectPreset(a.list[(pos+d+len(a.list))%len(a.list)], true)
}

func (a *App) startNaming() {
	a.naming = true
	a.nameBuf = a.Presets[a.cur].Name
	if !a.Presets[a.cur].User {
		a.nameBuf = "My " + a.nameBuf
	}
	a.say("Type a name, then Enter to save (Esc cancels)")
}

func (a *App) finishNaming(ok bool) {
	a.naming = false
	name := strings.TrimSpace(a.nameBuf)
	if !ok || name == "" {
		return
	}
	p := synth.Preset{Name: name, Cat: "User", Patch: a.Eng.Patch(), User: true}
	a.mu.Lock()
	p.Knobs = a.knobs
	a.mu.Unlock()
	for i := range a.Presets {
		if a.Presets[i].User && a.Presets[i].Name == name {
			a.Presets[i] = p
			a.cur = i
			a.say("Saved over " + name)
			a.rebuildList()
			a.Flush()
			return
		}
		if !a.Presets[i].User && a.Presets[i].Name == name {
			p.Name = name + " (mine)"
		}
	}
	a.Presets = append(a.Presets, p)
	a.cur = len(a.Presets) - 1
	a.rebuildList()
	a.say("Saved " + p.Name + " in the User category")
	a.Flush()
}

func (a *App) deleteUser() {
	if !a.Presets[a.cur].User {
		a.say("Only your own (User) presets can be deleted")
		return
	}
	name := a.Presets[a.cur].Name
	a.Presets = append(a.Presets[:a.cur], a.Presets[a.cur+1:]...)
	a.rebuildList()
	a.selectPreset(clampI(a.cur, 0, len(a.Presets)-1), false)
	a.say("Deleted " + name)
	a.Flush()
}

// ---------------------------------------------------------------- tempo / recording

func (a *App) nudgeBPM(d int, shift bool) {
	step := 1.0
	if shift {
		step = 10
	}
	a.Eng.SetBPM(math.Round(a.st.BPM) + float64(d)*step)
	a.markDirty()
}

func (a *App) tap() {
	now := time.Now()
	if len(a.taps) > 0 && now.Sub(a.taps[len(a.taps)-1]) > 2*time.Second {
		a.taps = a.taps[:0]
	}
	a.taps = append(a.taps, now)
	if len(a.taps) > 6 {
		a.taps = a.taps[len(a.taps)-6:]
	}
	if n := len(a.taps); n >= 2 {
		iv := a.taps[n-1].Sub(a.taps[0]).Seconds() / float64(n-1)
		a.Eng.SetBPM(math.Round(60/iv*10) / 10)
		a.markDirty()
	}
}

func (a *App) toggleRec() {
	if a.st.Recording {
		path, sec := a.Eng.StopRecording()
		a.lastRec = path
		a.say(fmt.Sprintf("Saved %s (%.0f s) in %s", filepath.Base(path), sec, filepath.Dir(path)))
		return
	}
	if a.RecDir == "" {
		a.say("No folder to record into")
		return
	}
	path := filepath.Join(a.RecDir, "WaveBass "+time.Now().Format("2006-01-02 15.04.05")+".wav")
	if err := a.Eng.StartRecording(path); err != nil {
		a.say("Can't record: " + err.Error())
		return
	}
	a.say("Recording to " + path)
}

// Panic silences everything.
func (a *App) Panic() {
	for i := range a.playing {
		a.playing[i].Store(0)
	}
	a.mouseNote = -1
	a.Eng.Panic()
	a.say("All notes off")
}

// ---------------------------------------------------------------- MIDI (called on the MIDI thread)

func (a *App) shiftMIDIOct(d int) {
	o := clampI(int(a.midiOct.Load())+d, -4, 4)
	a.midiOct.Store(int32(o))
	a.say(fmt.Sprintf("MIDI keyboard octave %+d", o))
	a.markDirty()
}

func reservedCC(cc byte) bool {
	switch {
	case cc == 0, cc == 1, cc == 6, cc == 32, cc == 38, cc == 64:
		return true
	case cc >= 96 && cc <= 101, cc >= 120:
		return true
	}
	return false
}

// HandleMIDI takes one raw MIDI message.
func (a *App) HandleMIDI(status, d1, d2 byte) {
	if status >= 0xF8 {
		switch status {
		case 0xF8:
			a.Eng.MIDIClock(time.Now())
		case 0xFA:
			a.Eng.MIDIStart()
		}
		return
	}
	if d1 > 127 || d2 > 127 {
		return
	}
	switch status & 0xF0 {
	case 0x90:
		if d2 > 0 {
			n := int(d1) + 12*int(a.midiOct.Load())
			if n < 0 || n > 127 {
				return
			}
			if old := a.playing[d1].Swap(int32(n) + 1); old > 0 && int(old-1) != n {
				a.Eng.KeyUp(int(old - 1))
			}
			a.midiNotes.Add(1)
			a.Eng.KeyDown(n, math.Pow(float64(d2)/127, 0.8))
			return
		}
		fallthrough
	case 0x80:
		if n := a.playing[d1].Swap(0) - 1; n >= 0 {
			a.Eng.KeyUp(int(n))
		}
	case 0xB0:
		a.midiNotes.Add(1)
		switch {
		case d1 == 1:
			a.Eng.SetWheel(float64(d2) / 127)
		case d1 == 64:
			a.Eng.SetPedal(d2 >= 64)
		case d1 == 120 || d1 == 123:
			for i := range a.playing {
				if n := a.playing[i].Swap(0) - 1; n >= 0 {
					a.Eng.KeyUp(int(n))
				}
			}
		case !reservedCC(d1):
			a.handleKnobCC(int(d1), int(d2))
		}
	case 0xC0:
		a.pendingProg.Store(int32(d1) + 1)
	case 0xD0:
		a.Eng.SetAftertouch(float64(d1) / 127)
	case 0xE0:
		a.Eng.SetBend((float64(int(d2)<<7|int(d1)) - 8192) / 8192)
	}
}

// handleKnobCC routes a controller to one of the eight knob slots. Unknown controllers claim
// the next free slot, so a new keyboard works by simply turning its knobs left to right.
func (a *App) handleKnobCC(cc, val int) {
	a.mu.Lock()
	a.lastCC, a.lastCCv = cc, val
	slot := -1
	if a.learn >= 0 {
		for i := range a.knobCC {
			if a.knobCC[i] == cc {
				a.knobCC[i] = -1
			}
		}
		slot = a.learn
		a.knobCC[slot] = cc
		a.learn = -1
	}
	for i := 0; slot < 0 && i < 8; i++ {
		if a.knobCC[i] == cc {
			slot = i
		}
	}
	for i := 0; slot < 0 && i < 8; i++ {
		if a.knobCC[i] < 0 {
			slot = i
			a.knobCC[i] = cc
		}
	}
	if slot < 0 {
		a.mu.Unlock()
		return
	}
	if a.pickOn {
		a.knobs[slot] = a.pickPID
		a.pickOn = false
	}
	id := a.knobs[slot]
	a.knobT[slot] = time.Now()
	a.mu.Unlock()
	a.Eng.SetNorm(id, float64(val)/127)
}

// ---------------------------------------------------------------- per-frame

// Frame is called once per UI frame.
func (a *App) Frame() {
	a.st = a.Eng.Snapshot()
	if pc := a.pendingProg.Swap(0); pc > 0 {
		a.selectPreset(int(pc-1)%len(a.Presets), true)
	}
	if n := a.midiNotes.Load(); n != a.midiSeen {
		a.midiSeen, a.midiFlash = n, time.Now()
	}
	if a.dirty && time.Since(a.dirtyT) > 1500*time.Millisecond {
		a.Flush()
	}
}

// NoteOn / NoteOff play from the computer keyboard (offset is semitones above the base key).
func (a *App) NoteOn(offset int) {
	if !a.naming {
		a.Eng.KeyDown(a.kbBase+offset, 0.8)
	}
}

func (a *App) NoteOff(offset int) { a.Eng.KeyUp(a.kbBase + offset) }

// Naming reports whether a preset name is being typed (keys go to the text box).
func (a *App) Naming() bool { return a.naming }

// Type adds typed characters to the preset name.
func (a *App) Type(r []rune) {
	for _, c := range r {
		if c >= 32 && c != 127 && len([]rune(a.nameBuf)) < 24 {
			a.nameBuf += string(c)
		}
	}
}

// Key handles a named key press: "enter", "esc", "backspace", "up", "down", "left", "right",
// "space".
func (a *App) Key(name string) {
	if a.naming {
		switch name {
		case "enter":
			a.finishNaming(true)
		case "esc":
			a.finishNaming(false)
		case "backspace":
			if r := []rune(a.nameBuf); len(r) > 0 {
				a.nameBuf = string(r[:len(r)-1])
			}
		}
		return
	}
	switch name {
	case "esc":
		a.mu.Lock()
		cancel := a.learn >= 0 || a.pickSlot >= 0 || a.pickOn
		a.learn, a.pickSlot, a.pickOn = -1, -1, false
		a.mu.Unlock()
		if !cancel {
			a.Panic()
		}
	case "up":
		a.stepPreset(-1)
	case "down":
		a.stepPreset(1)
	case "left", "right":
		d := 12
		if name == "left" {
			d = -12
		}
		if nb := a.kbBase + d; nb >= 0 && nb <= 84 {
			a.kbBase = nb
			a.say("Computer keys start at " + noteName(nb))
			a.markDirty()
		}
	case "space":
		if a.st.Loop.State >= 2 {
			a.Eng.LoopPlay()
		}
	}
}

func noteName(m int) string {
	names := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return fmt.Sprintf("%s%d", names[m%12], m/12-1)
}
