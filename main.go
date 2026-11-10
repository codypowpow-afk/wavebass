// WaveBass: a standalone wavetable bass synth for a USB MIDI keyboard.
package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/codypowpow-afk/wavebass/internal/appicon"
	"github.com/codypowpow-afk/wavebass/internal/ui"
	"github.com/codypowpow-afk/wavebass/synth"
)

// ---------------------------------------------------------------- drawing

type textOp struct {
	s     string
	f     ui.Font
	x, y  float32
	c     color.RGBA
	align int
}

// canvas batches the frame into three draw calls per layer: flat rectangles, anti-aliased
// shapes, then text.
type canvas struct {
	dst    *ebiten.Image
	white  *ebiten.Image
	rv, sv []ebiten.Vertex
	ri, si []uint16
	texts  []textOp
	faces  [6]text.Face
}

func newCanvas() *canvas {
	c := &canvas{}
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	c.white = img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
	src := func(b []byte) *text.GoTextFaceSource {
		s, err := text.NewGoTextFaceSource(bytes.NewReader(b))
		if err != nil {
			log.Fatal(err)
		}
		return s
	}
	reg, bold := src(goregular.TTF), src(gobold.TTF)
	c.faces[ui.FTiny] = &text.GoTextFace{Source: bold, Size: 9}
	c.faces[ui.FSmall] = &text.GoTextFace{Source: bold, Size: 11}
	c.faces[ui.FBody] = &text.GoTextFace{Source: reg, Size: 13}
	c.faces[ui.FBold] = &text.GoTextFace{Source: bold, Size: 14}
	c.faces[ui.FBig] = &text.GoTextFace{Source: bold, Size: 18}
	c.faces[ui.FTitle] = &text.GoTextFace{Source: bold, Size: 24}
	return c
}

func vert(x, y float32, c color.RGBA) ebiten.Vertex {
	return ebiten.Vertex{DstX: x, DstY: y, SrcX: 1, SrcY: 1,
		ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: 1}
}

func (c *canvas) Rect(x, y, w, h float32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	if len(c.rv) > 60000 {
		c.Layer()
	}
	b := uint16(len(c.rv))
	c.rv = append(c.rv, vert(x, y, col), vert(x+w, y, col), vert(x+w, y+h, col), vert(x, y+h, col))
	c.ri = append(c.ri, b, b+1, b+2, b, b+2, b+3)
}

func (c *canvas) roomForShape() {
	if len(c.sv) > 55000 {
		c.Layer()
	}
}

func (c *canvas) Circle(x, y, r float32, col color.RGBA) {
	c.roomForShape()
	n := int(r*2) + 10
	if n > 48 {
		n = 48
	}
	b := uint16(len(c.sv))
	c.sv = append(c.sv, vert(x, y, col))
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		c.sv = append(c.sv, vert(x+r*float32(math.Cos(a)), y+r*float32(math.Sin(a)), col))
	}
	for i := 0; i < n; i++ {
		c.si = append(c.si, b, b+1+uint16(i), b+1+uint16((i+1)%n))
	}
}

// Arc strokes part of a circle as one triangle strip.
func (c *canvas) Arc(cx, cy, r, a0, a1, width float32, col color.RGBA) {
	if a1 <= a0 {
		return
	}
	c.roomForShape()
	n := int((a1-a0)*r/3) + 2
	ri, ro := r-width/2, r+width/2
	b := uint16(len(c.sv))
	for i := 0; i <= n; i++ {
		a := float64(a0) + float64(a1-a0)*float64(i)/float64(n)
		cs, sn := float32(math.Cos(a)), float32(math.Sin(a))
		c.sv = append(c.sv, vert(cx+cs*ri, cy+sn*ri, col), vert(cx+cs*ro, cy+sn*ro, col))
	}
	for i := 0; i < n; i++ {
		k := b + uint16(2*i)
		c.si = append(c.si, k, k+1, k+2, k+1, k+3, k+2)
	}
}

func (c *canvas) Polyline(pts []float32, width float32, col color.RGBA) {
	if len(pts) < 4 {
		return
	}
	c.roomForShape()
	if len(pts) == 4 { // a single line: one quad
		dx, dy := pts[2]-pts[0], pts[3]-pts[1]
		l := float32(math.Hypot(float64(dx), float64(dy)))
		if l == 0 {
			return
		}
		nx, ny := -dy/l*width/2, dx/l*width/2
		b := uint16(len(c.sv))
		c.sv = append(c.sv, vert(pts[0]+nx, pts[1]+ny, col), vert(pts[2]+nx, pts[3]+ny, col),
			vert(pts[2]-nx, pts[3]-ny, col), vert(pts[0]-nx, pts[1]-ny, col))
		c.si = append(c.si, b, b+1, b+2, b, b+2, b+3)
		return
	}
	var p vector.Path
	p.MoveTo(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		p.LineTo(pts[i], pts[i+1])
	}
	n := len(c.sv)
	c.sv, c.si = p.AppendVerticesAndIndicesForStroke(c.sv, c.si, &vector.StrokeOptions{Width: width, LineJoin: vector.LineJoinRound})
	for i := n; i < len(c.sv); i++ {
		v := &c.sv[i]
		v.SrcX, v.SrcY = 1, 1
		v.ColorR, v.ColorG, v.ColorB, v.ColorA = float32(col.R)/255, float32(col.G)/255, float32(col.B)/255, 1
	}
}

func (c *canvas) Text(s string, f ui.Font, x, y float32, col color.RGBA, align int) {
	if s != "" {
		c.texts = append(c.texts, textOp{s, f, x, y, col, align})
	}
}

// Layer draws everything queued so far.
func (c *canvas) Layer() {
	if len(c.ri) > 0 {
		c.dst.DrawTriangles(c.rv, c.ri, c.white, nil)
	}
	if len(c.si) > 0 {
		// Every shape here is a fan, strip or stroke without overlaps, so "fill all" is right.
		op := &ebiten.DrawTrianglesOptions{AntiAlias: true}
		c.dst.DrawTriangles(c.sv, c.si, c.white, op)
	}
	for i := range c.texts {
		t := &c.texts[i]
		op := &text.DrawOptions{}
		op.GeoM.Translate(float64(t.x), float64(t.y))
		op.ColorScale.ScaleWithColor(t.c)
		switch t.align {
		case ui.AlignC:
			op.PrimaryAlign = text.AlignCenter
		case ui.AlignR:
			op.PrimaryAlign = text.AlignEnd
		}
		text.Draw(c.dst, t.s, c.faces[t.f], op)
	}
	c.rv, c.ri, c.sv, c.si, c.texts = c.rv[:0], c.ri[:0], c.sv[:0], c.si[:0], c.texts[:0]
}

// ---------------------------------------------------------------- window

// Computer-keyboard piano: two rows, like most DAWs. Offsets are semitones above the base key.
var noteKeys = map[ebiten.Key]int{
	ebiten.KeyZ: 0, ebiten.KeyS: 1, ebiten.KeyX: 2, ebiten.KeyD: 3, ebiten.KeyC: 4, ebiten.KeyV: 5,
	ebiten.KeyG: 6, ebiten.KeyB: 7, ebiten.KeyH: 8, ebiten.KeyN: 9, ebiten.KeyJ: 10, ebiten.KeyM: 11,
	ebiten.KeyComma: 12, ebiten.KeyL: 13, ebiten.KeyPeriod: 14, ebiten.KeySemicolon: 15, ebiten.KeySlash: 16,
	ebiten.KeyQ: 12, ebiten.Key2: 13, ebiten.KeyW: 14, ebiten.Key3: 15, ebiten.KeyE: 16, ebiten.KeyR: 17,
	ebiten.Key5: 18, ebiten.KeyT: 19, ebiten.Key6: 20, ebiten.KeyY: 21, ebiten.Key7: 22, ebiten.KeyU: 23,
	ebiten.KeyI: 24, ebiten.Key9: 25, ebiten.KeyO: 26, ebiten.Key0: 27, ebiten.KeyP: 28,
	ebiten.KeyBracketLeft: 29, ebiten.KeyEqual: 30, ebiten.KeyBracketRight: 31,
}

var namedKeys = map[ebiten.Key]string{
	ebiten.KeyEnter: "enter", ebiten.KeyNumpadEnter: "enter", ebiten.KeyEscape: "esc", ebiten.KeyBackspace: "backspace",
	ebiten.KeyArrowUp: "up", ebiten.KeyArrowDown: "down", ebiten.KeyArrowLeft: "left", ebiten.KeyArrowRight: "right",
	ebiten.KeySpace: "space", ebiten.KeyPageUp: "up", ebiten.KeyPageDown: "down",
}

type game struct {
	app   *ui.App
	c     *canvas
	held  map[ebiten.Key]int
	runes []rune
}

func (g *game) Update() error {
	a := g.app
	a.Frame()
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	mx, my := ebiten.CursorPosition()
	x, y := float32(mx), float32(my)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.MouseDown(x, y, false, shift)
	} else if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		a.MouseMove(x, y, shift)
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		a.MouseUp()
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		a.MouseDown(x, y, true, shift)
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		a.Wheel(x, y, wy, shift)
	}

	g.runes = ebiten.AppendInputChars(g.runes[:0])
	if a.Naming() {
		a.Type(g.runes)
	}
	for k, name := range namedKeys {
		d := inpututil.KeyPressDuration(k)
		if d == 1 || (d > 25 && d%4 == 0 && (name == "backspace" || name == "up" || name == "down")) {
			a.Key(name)
		}
	}
	// Notes from the computer keyboard.
	if !a.Naming() {
		for k, off := range noteKeys {
			if inpututil.IsKeyJustPressed(k) {
				if _, ok := g.held[k]; !ok {
					g.held[k] = off
					a.NoteOn(off)
				}
			}
		}
	}
	for k, off := range g.held {
		if !ebiten.IsKeyPressed(k) || !ebiten.IsFocused() {
			delete(g.held, k)
			a.NoteOff(off)
		}
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.c.dst = screen
	g.app.Draw(g.c)
	g.c.Layer()
}

func (g *game) Layout(w, h int) (int, int) { return ui.W, ui.H }

// ---------------------------------------------------------------- settings

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "WaveBass", "config.json")
}

func loadConfig() ui.Config {
	cfg := ui.DefaultConfig()
	p := configPath()
	if p == "" {
		return cfg
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	loaded := ui.DefaultConfig()
	if json.Unmarshal(b, &loaded) != nil {
		return cfg
	}
	return loaded
}

func saveConfig(c *ui.Config) {
	p := configPath()
	if p == "" {
		return
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, p)
	}
}

// recordingDir is where WAV recordings go: Music\WaveBass, or next to the exe.
func recordingDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(home, "Music", "WaveBass")
		if os.MkdirAll(d, 0o755) == nil {
			return d
		}
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return ""
}

func main() {
	eng := synth.NewEngine()
	app := ui.New(eng, loadConfig())
	app.SaveConfig = saveConfig
	app.RecDir = recordingDir()
	app.OpenFolder = openFolder

	ctx := audio.NewContext(synth.SampleRate)
	player, err := ctx.NewPlayerF32(eng)
	if err != nil {
		log.Fatal(err)
	}
	player.SetBufferSize(20 * time.Millisecond) // low latency for playing live
	player.Play()

	midi := startMIDI(app.HandleMIDI)
	app.Devices = midi.Devices
	app.Rescan = midi.Rescan

	// Fit the window on small or scaled displays.
	w, h := ui.W, ui.H
	if sw, sh := ebiten.ScreenSizeInFullscreen(); sw > 0 && sh > 0 {
		for _, lim := range [][2]int{{sw - 30, ui.W}, {sh - 90, ui.H}} {
			if s := float64(lim[0]) / float64(lim[1]); s < float64(w)/float64(ui.W) {
				w, h = int(float64(ui.W)*s), int(float64(ui.H)*s)
			}
		}
	}
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle("WaveBass")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowIcon([]image.Image{appicon.Image(16), appicon.Image(32), appicon.Image(48)})
	ebiten.SetTPS(120)
	g := &game{app: app, c: newCanvas(), held: map[ebiten.Key]int{}}
	if err := ebiten.RunGame(g); err != nil {
		log.Print(err)
	}
	eng.StopRecording()
	app.Flush()
	midi.Close()
}
