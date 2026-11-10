package ui

import (
	"math"
	"time"

	"github.com/codypowpow-afk/wavebass/synth"
)

func in(x, y, rx, ry, rw, rh float32) bool { return x >= rx && x < rx+rw && y >= ry && y < ry+rh }

func (a *App) setParam(id synth.PID, v float64) {
	a.Eng.Set(id, v)
	if synth.Metas[id].Global {
		a.markDirty()
	}
}

func (a *App) stepParam(id synth.PID, d int) {
	m := &synth.Metas[id]
	v := a.Eng.Get(id) + float64(d)
	if m.Kind == synth.KChoice {
		n := m.Max + 1
		v = math.Mod(v+n, n)
	}
	a.setParam(id, v)
}

func (a *App) nudgeParam(id synth.PID, d float64) {
	m := &synth.Metas[id]
	if m.Stepped() {
		if d > 0 {
			a.stepParam(id, 1)
		} else {
			a.stepParam(id, -1)
		}
		return
	}
	a.setParam(id, m.FromNorm(m.ToNorm(a.Eng.Get(id))+d))
}

// hwSlotAt returns which hardware-knob slot is under the point and which part of it:
// 0 = the knob, 1 = the CC label, 2 = the target name.
func hwSlotAt(x, y float32) (slot, part int) {
	if y < hwY+24 || y >= hwY+hwH-6 || x < mainX+8 || x >= mainX+mainW-8 {
		return -1, 0
	}
	sw := float32(mainW-16) / 8
	slot = int((x - (mainX + 8)) / sw)
	lx := x - (mainX + 8) - float32(slot)*sw
	ly := y - (hwY + 24)
	switch {
	case ly >= 60:
		return slot, 2
	case lx >= 60 && ly < 22:
		return slot, 1
	}
	return slot, 0
}

func keyAt(x, y float32) int {
	if y < kbY || y >= kbY+kbH {
		return -1
	}
	for m := kbLow; m <= kbHigh; m++ {
		if isBlack(m) {
			if kx, ky, kw, kh := keyRect(m); in(x, y, kx, ky, kw, kh) {
				return m
			}
		}
	}
	for m := kbLow; m <= kbHigh; m++ {
		if !isBlack(m) {
			if kx, ky, kw, kh := keyRect(m); in(x, y, kx, ky, kw, kh) {
				return m
			}
		}
	}
	return -1
}

func (a *App) widgetAt(x, y float32) *widget {
	for i := range a.widgets {
		w := &a.widgets[i]
		if in(x, y, w.x, w.y, w.w, w.h) {
			return w
		}
	}
	return nil
}

// MouseDown handles a button press. right = right button, shift = Shift held.
func (a *App) MouseDown(x, y float32, right, shift bool) {
	if a.naming {
		return
	}
	now := time.Now()
	double := now.Sub(a.lastClk) < 350*time.Millisecond
	a.lastClk = now

	for i := range a.buttons {
		b := &a.buttons[i]
		if in(x, y, b.x, b.y, b.w, b.h) {
			if !right {
				b.act(a, shift)
			}
			return
		}
	}
	// Tempo box: drag to change.
	if in(x, y, 750, 12, 74, 34) {
		a.dragBPM, a.dragY, a.dragN = true, y, a.st.BPM
		return
	}
	// Preset name box: click steps forward.
	if in(x, y, 230, 12, 326, 34) {
		if right {
			a.stepPreset(-1)
		} else {
			a.stepPreset(1)
		}
		return
	}
	// Browser.
	if in(x, y, browX, browListY, browW, browRows*browRowH) {
		row := int((y-browListY)/browRowH) + a.scroll
		if row >= 0 && row < len(a.list) {
			a.selectPreset(a.list[row], false)
		}
		return
	}
	// Controls.
	if w := a.widgetAt(x, y); w != nil {
		m := &synth.Metas[w.pid]
		a.mu.Lock()
		ps := a.pickSlot
		a.mu.Unlock()
		if ps >= 0 || right {
			if m.NoKnob {
				a.say("That one can't go on a knob")
				return
			}
			a.mu.Lock()
			if ps >= 0 {
				a.knobs[ps] = w.pid
				a.pickSlot = -1
			} else {
				a.pickPID, a.pickOn = w.pid, true
				a.learn = -1
			}
			a.mu.Unlock()
			if ps >= 0 {
				a.say("Knob " + string(rune('1'+ps)) + " now controls " + m.Long)
			} else {
				a.say("Now turn the hardware knob you want for " + m.Long + " (Esc cancels)")
			}
			a.markDirty()
			return
		}
		switch w.kind {
		case wKnob:
			if double && a.lastW == w {
				a.setParam(w.pid, m.Def)
			} else {
				a.drag, a.dragY, a.dragN = w, y, m.ToNorm(a.Eng.Get(w.pid))
			}
		case wSlider:
			if double && a.lastW == w {
				a.setParam(w.pid, m.Def)
			} else {
				a.drag = w
				a.setParam(w.pid, m.FromNorm(float64((x-w.x)/w.w)))
			}
		case wToggle:
			a.stepParam(w.pid, 1)
		default:
			if x < w.x+w.w*0.35 {
				a.stepParam(w.pid, -1)
			} else {
				a.stepParam(w.pid, 1)
			}
		}
		a.lastW = w
		return
	}
	a.lastW = nil
	// The eight hardware knob slots.
	if slot, part := hwSlotAt(x, y); slot >= 0 {
		a.mu.Lock()
		id := a.knobs[slot]
		switch part {
		case 1:
			if a.learn == slot {
				a.learn = -1
			} else {
				a.learn, a.pickSlot, a.pickOn = slot, -1, false
			}
		case 2:
			if a.pickSlot == slot {
				a.pickSlot = -1
			} else {
				a.pickSlot, a.learn, a.pickOn = slot, -1, false
			}
		}
		learn, pick := a.learn, a.pickSlot
		a.mu.Unlock()
		switch part {
		case 0:
			m := &synth.Metas[id]
			if m.Stepped() {
				a.stepParam(id, 1)
			} else {
				a.dragHW, a.dragY, a.dragN = slot, y, m.ToNorm(a.Eng.Get(id))
			}
		case 1:
			if learn == slot {
				a.say("Turn the knob on your keyboard that should be knob " + string(rune('1'+slot)))
			}
		case 2:
			if pick == slot {
				a.say("Click any control on screen to put it on knob " + string(rune('1'+slot)) + " (or scroll here)")
			}
		}
		return
	}
	// On-screen keyboard.
	if n := keyAt(x, y); n >= 0 {
		a.mouseNote = n
		a.Eng.KeyDown(n, 0.8)
	}
}

// MouseMove is called while the left button is held.
func (a *App) MouseMove(x, y float32, shift bool) {
	scale := 1.0 / 180
	if shift {
		scale /= 6
	}
	switch {
	case a.dragBPM:
		a.Eng.SetBPM(math.Round(a.dragN + float64(a.dragY-y)/3))
		a.markDirty()
	case a.drag != nil:
		m := &synth.Metas[a.drag.pid]
		if a.drag.kind == wSlider {
			a.setParam(a.drag.pid, m.FromNorm(float64((x-a.drag.x)/a.drag.w)))
		} else {
			a.setParam(a.drag.pid, m.FromNorm(a.dragN+float64(a.dragY-y)*scale))
		}
	case a.dragHW >= 0:
		a.mu.Lock()
		id := a.knobs[a.dragHW]
		a.mu.Unlock()
		a.setParam(id, synth.Metas[id].FromNorm(a.dragN+float64(a.dragY-y)*scale))
	case a.mouseNote >= 0:
		if n := keyAt(x, y); n != a.mouseNote && n >= 0 {
			a.Eng.KeyUp(a.mouseNote)
			a.mouseNote = n
			a.Eng.KeyDown(n, 0.8)
		}
	}
}

// MouseUp ends a drag.
func (a *App) MouseUp() {
	a.drag, a.dragHW, a.dragBPM = nil, -1, false
	if a.mouseNote >= 0 {
		a.Eng.KeyUp(a.mouseNote)
		a.mouseNote = -1
	}
}

// knobTargets lists every parameter a hardware knob may control.
func knobTargets() []synth.PID {
	var t []synth.PID
	for i := range synth.Metas {
		if !synth.Metas[i].NoKnob {
			t = append(t, synth.PID(i))
		}
	}
	return t
}

// Wheel handles the scroll wheel (dy > 0 = up).
func (a *App) Wheel(x, y float32, dy float64, shift bool) {
	if dy == 0 || a.naming {
		return
	}
	d := 1
	if dy < 0 {
		d = -1
	}
	if in(x, y, browX, browY, browW, H-browY) {
		a.scroll = clampI(a.scroll-d*3, 0, maxI(0, len(a.list)-browRows))
		return
	}
	if in(x, y, 722, 12, 130, 34) {
		a.nudgeBPM(d, shift)
		return
	}
	if in(x, y, 196, 12, 394, 34) {
		a.stepPreset(-d)
		return
	}
	if w := a.widgetAt(x, y); w != nil {
		step := 0.02
		if shift {
			step = 0.004
		}
		a.nudgeParam(w.pid, float64(d)*step)
		return
	}
	if slot, part := hwSlotAt(x, y); slot >= 0 {
		a.mu.Lock()
		id := a.knobs[slot]
		if part == 2 { // scroll through what the knob controls
			t := knobTargets()
			for i, p := range t {
				if p == id {
					a.knobs[slot] = t[(i-d+len(t))%len(t)]
					break
				}
			}
			a.mu.Unlock()
			a.markDirty()
			return
		}
		a.mu.Unlock()
		a.nudgeParam(id, float64(d)*0.02)
	}
}
