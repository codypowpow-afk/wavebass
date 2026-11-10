package synth

import (
	"math"
	"sort"
)

// ---------------------------------------------------------------- arpeggiator / note repeat

type arpNote struct {
	note  int
	vel   float64
	order int64
}

type arpOut struct {
	note int
	off  float64 // beat at which the note is released
}

type arpState struct {
	held, latched []arpNote
	order         int64
	running       bool
	step          int
	last          float64 // un-swung beat of the previous step
	out           []arpOut
	lastRand      int
	scratch       []arpNote
}

func (a *arpState) init() { a.lastRand = -1 }

func putNote(list []arpNote, n arpNote) []arpNote {
	for i := range list {
		if list[i].note == n.note {
			list[i] = n
			return list
		}
	}
	return append(list, n)
}

func (a *arpState) press(e *Engine, note int, vel float64) {
	if len(a.held) == 0 {
		a.latched = a.latched[:0] // a fresh chord replaces the latched one
	}
	a.order++
	n := arpNote{note, vel, a.order}
	a.held = putNote(a.held, n)
	a.latched = putNote(a.latched, n)
}

func (a *arpState) release(note int) {
	for i := range a.held {
		if a.held[i].note == note {
			a.held = append(a.held[:i], a.held[i+1:]...)
			return
		}
	}
}

func (a *arpState) pool(e *Engine) []arpNote {
	if e.P[PArpLatch] >= 0.5 || e.pedal {
		return a.latched
	}
	return a.held
}

func (a *arpState) unlatch() { a.latched = append(a.latched[:0], a.held...) }

func (a *arpState) clear() {
	a.held, a.latched, a.out = a.held[:0], a.latched[:0], a.out[:0]
	a.running = false
}

// flush releases every note the arp is currently sounding.
func (a *arpState) flush(e *Engine) {
	for _, o := range a.out {
		e.releaseNote(o.note, false)
	}
	a.out = a.out[:0]
}

func (a *arpState) tick(e *Engine) {
	j := 0
	for _, o := range a.out {
		if e.beat >= o.off {
			e.releaseNote(o.note, false)
		} else {
			a.out[j] = o
			j++
		}
	}
	a.out = a.out[:j]
	mode := int(e.P[PArpMode])
	if mode == ArpOff {
		return
	}
	pool := a.pool(e)
	if len(pool) == 0 {
		a.running = false
		return
	}
	stepLen := ArpRates[int(e.P[PArpRate])].Beats
	locked := e.gridLocked()
	if !a.running {
		a.running, a.step = true, 0
		a.last = e.beat - stepLen
		if locked {
			// A key pressed just after the beat still lands on it; otherwise wait for the next step.
			g := math.Floor(e.beat/stepLen) * stepLen
			if e.beat-g > 0.35*stepLen {
				g += stepLen
			}
			a.last = g - stepLen
		}
	}
	next := a.last + stepLen
	odd := a.step&1 == 1
	if locked {
		next = math.Round(next/stepLen) * stepLen
		if next <= a.last+0.25*stepLen {
			next += stepLen
		}
		odd = int64(math.Round(next/stepLen))&1 == 1
	}
	if next < e.beat-2*stepLen { // fell behind (tempo jump): catch up
		next = e.beat
	}
	when := next
	if odd {
		when += e.P[PArpSwing] * stepLen * 0.5
	}
	if e.beat < when {
		return
	}
	gate := e.P[PArpGate] * stepLen
	for _, n := range a.pick(e, pool, mode) {
		if n.note < 0 || n.note > 127 {
			continue
		}
		e.trigger(n.note, n.vel, false)
		a.out = append(a.out, arpOut{n.note, e.beat + gate})
	}
	a.step++
	a.last = next
}

func (a *arpState) pick(e *Engine, pool []arpNote, mode int) []arpNote {
	octs := int(e.P[PArpOct])
	if octs < 1 {
		octs = 1
	}
	s := append(a.scratch[:0], pool...)
	a.scratch = s
	if mode == ArpRepeat {
		up := 12 * (a.step % octs)
		for i := range s {
			s[i].note += up
		}
		return s
	}
	if mode == ArpPlayed {
		sort.Slice(s, func(i, j int) bool { return s[i].order < s[j].order })
	} else {
		sort.Slice(s, func(i, j int) bool { return s[i].note < s[j].note })
	}
	n := len(s)
	L := n * octs
	var i int
	switch mode {
	case ArpDown:
		i = L - 1 - a.step%L
	case ArpUpDown:
		period := 2*L - 2
		if period < 1 {
			period = 1
		}
		k := a.step % period
		if k < L {
			i = k
		} else {
			i = period - k
		}
	case ArpRandom:
		i = int(e.rand() * float64(L))
		if L > 1 && i == a.lastRand {
			i = (i + 1) % L
		}
		a.lastRand = i
	default:
		i = a.step % L
	}
	if i >= L {
		i = L - 1
	}
	out := s[i%n]
	out.note += 12 * (i / n)
	for out.note > 127 {
		out.note -= 12
	}
	s[0] = out
	return s[:1]
}

// ---------------------------------------------------------------- loop recorder

const (
	loopEmpty = iota
	loopArmed
	loopRecording
	loopPlaying
	loopStopped
)

type loopEv struct {
	beat  float64
	note  int
	vel   float64
	on    bool
	fresh bool // recorded on this pass: don't play it back until the loop comes round
}

type looper struct {
	state    int
	overdub  bool
	quant    bool
	bars     int
	length   float64
	start    float64
	pos      float64
	ev       []loopEv
	sounding [128]bool
	recOn    [128]bool
	recShift [128]float64
}

// LoopStatus is the loop recorder's state for the UI.
type LoopStatus struct {
	State   int // 0 empty, 1 armed, 2 recording, 3 playing, 4 stopped
	Overdub bool
	Quant   bool
	Bars    int
	Pos     float64 // 0..1 through the loop
	Notes   int
}

func (l *looper) status(e *Engine) LoopStatus {
	st := LoopStatus{State: l.state, Overdub: l.overdub, Quant: l.quant, Bars: l.bars}
	for _, ev := range l.ev {
		if ev.on {
			st.Notes++
		}
	}
	switch l.state {
	case loopRecording:
		st.Pos = clamp((e.beat-l.start)/l.length, 0, 1)
	case loopPlaying:
		st.Pos = clamp(l.pos/l.length, 0, 1)
	}
	return st
}

func (l *looper) wrap(off float64) float64 {
	off = math.Mod(off, l.length)
	if off < 0 {
		off += l.length
	}
	return off
}

func (l *looper) record(e *Engine, note int, vel float64, on bool) {
	switch l.state {
	case loopArmed:
		if !on {
			return
		}
		// The first note starts the loop. Line the transport up so it lands on a beat.
		if e.metro || e.extActive() {
			l.start = math.Round(e.beat)
		} else {
			d := math.Round(e.beat) - e.beat
			e.beat += d
			e.arp.last += d
			for i := range e.arp.out {
				e.arp.out[i].off += d
			}
			e.lastBt = int64(math.Floor(e.beat))
			l.start = e.beat
		}
		l.length = float64(l.bars) * 4
		l.ev = l.ev[:0]
		l.recOn = [128]bool{}
		l.state = loopRecording
	case loopRecording:
	case loopPlaying:
		if !l.overdub {
			return
		}
	default:
		return
	}
	off := l.wrap(math.Max(0, e.beat-l.start))
	if on {
		shift := 0.0
		if l.quant {
			q := math.Round(off/0.25) * 0.25
			shift = q - off
			off = l.wrap(q)
		}
		l.recOn[note], l.recShift[note] = true, shift
		l.ev = append(l.ev, loopEv{off, note, vel, true, true})
	} else {
		if !l.recOn[note] {
			return
		}
		l.recOn[note] = false
		l.ev = append(l.ev, loopEv{l.wrap(off + l.recShift[note]), note, 0, false, true})
	}
	l.sort()
}

func (l *looper) sort() {
	sort.SliceStable(l.ev, func(i, j int) bool {
		if l.ev[i].beat != l.ev[j].beat {
			return l.ev[i].beat < l.ev[j].beat
		}
		return !l.ev[i].on && l.ev[j].on
	})
}

// closeHanging ends notes that are still held when recording stops.
func (l *looper) closeHanging(at float64) {
	for n := range l.recOn {
		if l.recOn[n] {
			l.recOn[n] = false
			l.ev = append(l.ev, loopEv{at, n, 0, false, false})
		}
	}
	l.sort()
}

func (l *looper) finish(e *Engine) {
	l.closeHanging(l.length - 0.002)
	for i := range l.ev {
		l.ev[i].fresh = false
	}
	l.state, l.overdub = loopPlaying, false
	l.pos = -1e-9
}

func (l *looper) silence(e *Engine) {
	for n := range l.sounding {
		if l.sounding[n] {
			l.sounding[n] = false
			e.releaseNote(n, true)
		}
	}
}

func (l *looper) stop(e *Engine) {
	l.silence(e)
	l.overdub = false
	l.recOn = [128]bool{}
	if len(l.ev) > 0 && l.state != loopArmed {
		if l.state == loopRecording {
			l.closeHanging(l.wrap(e.beat - l.start))
			for i := range l.ev {
				l.ev[i].fresh = false
			}
		}
		l.state = loopStopped
	} else {
		l.state = loopEmpty
	}
}

func (l *looper) play(e *Engine, from, to float64) {
	for i := range l.ev {
		ev := &l.ev[i]
		if ev.fresh || ev.beat <= from || ev.beat > to {
			continue
		}
		if ev.on {
			if l.sounding[ev.note] {
				e.releaseNote(ev.note, true)
			}
			l.sounding[ev.note] = true
			e.trigger(ev.note, ev.vel, true)
		} else if l.sounding[ev.note] {
			l.sounding[ev.note] = false
			e.releaseNote(ev.note, true)
		}
	}
}

func (l *looper) tick(e *Engine) {
	switch l.state {
	case loopRecording:
		if e.beat-l.start >= l.length {
			l.finish(e)
		}
	case loopPlaying:
		cur := l.wrap(e.beat - l.start)
		if cur < l.pos {
			l.play(e, l.pos, l.length+1)
			l.silence(e)
			for i := range l.ev {
				l.ev[i].fresh = false
			}
			l.play(e, -1, cur)
		} else {
			l.play(e, l.pos, cur)
		}
		l.pos = cur
	}
}

// LoopRec is the loop recorder's record button: arm → (play to start) → loop → overdub on/off.
func (e *Engine) LoopRec() {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := &e.loop
	switch l.state {
	case loopEmpty:
		l.state = loopArmed
	case loopArmed:
		l.state = loopEmpty
	case loopRecording:
		// Finish at the end of the current bar.
		l.length = math.Max(4, math.Ceil((e.beat-l.start)/4)*4)
	case loopPlaying:
		l.overdub = !l.overdub
		if !l.overdub {
			l.closeHanging(l.pos)
		}
	case loopStopped:
		l.start, l.pos = e.beat, -1e-9
		l.state, l.overdub = loopPlaying, true
	}
}

// LoopPlay starts or stops loop playback.
func (e *Engine) LoopPlay() {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := &e.loop
	switch l.state {
	case loopPlaying, loopRecording:
		l.stop(e)
	case loopStopped:
		if e.metro || e.extActive() {
			l.pos = l.wrap(e.beat - l.start) // stay in step with the click
		} else {
			l.start, l.pos = e.beat, -1e-9
		}
		l.state = loopPlaying
	}
}

// LoopClear erases the loop.
func (e *Engine) LoopClear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loop.silence(e)
	e.loop.ev = e.loop.ev[:0]
	e.loop.state, e.loop.overdub = loopEmpty, false
	e.loop.recOn = [128]bool{}
}

// SetLoopBars sets the loop length (only before recording).
func (e *Engine) SetLoopBars(n int) {
	e.mu.Lock()
	if e.loop.state == loopEmpty || e.loop.state == loopArmed {
		e.loop.bars = n
	}
	e.mu.Unlock()
}

// SetLoopQuant turns 1/16 quantising of recorded notes on or off.
func (e *Engine) SetLoopQuant(on bool) { e.mu.Lock(); e.loop.quant = on; e.mu.Unlock() }
