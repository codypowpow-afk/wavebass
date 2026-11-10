package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/codypowpow-afk/wavebass/synth"
)

func dimmed(c color.RGBA, f float32) color.RGBA {
	return color.RGBA{uint8(float32(c.R) * f), uint8(float32(c.G) * f), uint8(float32(c.B) * f), 0xff}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

const (
	knobA0    = 0.75 * math.Pi
	knobSweep = 1.5 * math.Pi
)

// Draw paints the whole window.
func (a *App) Draw(c Canvas) {
	c.Rect(0, 0, W, H, cBG)
	for _, p := range a.panels {
		c.Rect(p.x, p.y, p.w, p.h, cPanel)
		if p.title != "" {
			c.Text(p.title, FTiny, p.x+8, p.y+5, cDim, AlignL)
		}
	}
	a.drawTop(c)
	a.drawBrowser(c)
	a.mu.Lock()
	knobs, ccs, learn, pickSlot, pickOn, pickPID := a.knobs, a.knobCC, a.learn, a.pickSlot, a.pickOn, a.pickPID
	knobT := a.knobT
	lastCC, lastCCv := a.lastCC, a.lastCCv
	a.mu.Unlock()
	for i := range a.widgets {
		w := &a.widgets[i]
		slot := -1
		for k, id := range knobs {
			if id == w.pid {
				slot = k
			}
		}
		a.drawWidget(c, w, slot, pickOn && pickPID == w.pid)
	}
	a.drawScopes(c)
	a.drawLFOs(c)
	a.drawHW(c, knobs, ccs, knobT, learn, pickSlot, lastCC, lastCCv)
	a.drawLoop(c)
	a.drawKeyboard(c)
	for i := range a.buttons {
		b := &a.buttons[i]
		on := b.on(a)
		bg, tc := cPanelHi, cText
		if on {
			bg, tc = b.col, cBG
		}
		c.Rect(b.x, b.y, b.w, b.h, bg)
		f := FSmall
		if b.h < 20 {
			f = FTiny
		}
		c.Text(b.label(a), f, b.x+b.w/2, b.y+b.h/2-6, tc, AlignC)
	}
	if a.toast != "" && time.Since(a.toastT) < 3500*time.Millisecond {
		c.Layer()
		n := float32(len([]rune(a.toast)))
		w := n*8.2 + 40
		if w > W-40 {
			w = W - 40
		}
		x := (W - w) / 2
		c.Rect(x, 68, w, 34, cAccent)
		c.Rect(x+2, 70, w-4, 30, cWell)
		c.Text(clip(a.toast, 165), FBold, W/2, 77, cText, AlignC)
	}
}

func (a *App) drawTop(c Canvas) {
	c.Rect(0, 0, W, 56, cPanel)
	c.Text("WAVEBASS", FTitle, 16, 13, cText, AlignL)
	c.Rect(16, 44, 152, 3, cAccent)
	// Preset name.
	p := &a.Presets[a.cur]
	c.Rect(230, 12, 326, 34, cWell)
	cc := catColor[p.Cat]
	c.Rect(230, 12, 4, 34, cc)
	if a.naming {
		cur := ""
		if time.Now().UnixMilli()/400%2 == 0 {
			cur = "_"
		}
		c.Text(a.nameBuf+cur, FBig, 244, 19, cYellow, AlignL)
	} else {
		c.Text(clip(p.Name, 22), FBig, 244, 19, cText, AlignL)
		c.Text(strings.ToUpper(p.Cat), FTiny, 548, 24, cc, AlignR)
	}
	// Tempo.
	c.Rect(750, 12, 74, 34, cWell)
	bc := cText
	if a.st.Ext {
		bc = cAccent2
	}
	c.Text(fmt.Sprintf("%.1f", a.st.BPM), FBig, 787, 14, bc, AlignC)
	c.Text("BPM", FTiny, 787, 34, cDim, AlignC)
	// beat light
	ph := a.st.Beat - math.Floor(a.st.Beat)
	if ph < 0.25 {
		lc := cAccent
		if int64(math.Floor(a.st.Beat))%4 == 0 {
			lc = cText
		}
		c.Circle(819, 17, 3, lc)
	}
	// MIDI status.
	x := float32(1210)
	var open, busy []string
	if a.Devices != nil {
		open, busy = a.Devices()
	}
	dot := cLine
	msg, mc := "No MIDI keyboard found", cDim
	if len(open) > 0 {
		dot, msg, mc = dimmed(cGreen, 0.6), strings.Join(open, " + "), cText
		if time.Since(a.midiFlash) < 120*time.Millisecond {
			dot = cGreen
		}
	} else if len(busy) > 0 {
		msg, mc = busy[0]+": in use by another app", cYellow
	}
	c.Circle(x+5, 17, 5, dot)
	c.Text(clip(msg, 30), FSmall, x+16, 10, mc, AlignL)
	// level meter
	c.Rect(x, 35, 116, 8, cWell)
	lv := float32(math.Min(1, a.st.Peak))
	mcol := cAccent2
	if a.st.Peak > 0.9 {
		mcol = cRed
	}
	c.Rect(x, 35, 116*lv, 8, mcol)
	c.Text(fmt.Sprintf("OCT %+d", a.midiOct.Load()), FSmall, 1383, 32, cText, AlignC)
}

func (a *App) drawBrowser(c Canvas) {
	name := "ALL SOUNDS"
	col := cText
	if a.cat >= 0 {
		name = strings.ToUpper(synth.Categories[a.cat])
		col = catColor[synth.Categories[a.cat]]
	}
	c.Text(name, FBold, browX+browW/2, browY+10, col, AlignC)
	for row := 0; row < browRows; row++ {
		i := row + a.scroll
		if i >= len(a.list) {
			break
		}
		idx := a.list[i]
		p := &a.Presets[idx]
		y := float32(browListY + row*browRowH)
		tc := cDim
		if idx == a.cur {
			c.Rect(browX+4, y, browW-8, browRowH-1, cPanelHi)
			tc = cText
		}
		c.Rect(browX+8, y+6, 4, browRowH-13, catColor[p.Cat])
		c.Text(clip(p.Name, 24), FBody, browX+20, y+3, tc, AlignL)
	}
	if len(a.list) > browRows {
		// scroll bar
		h := float32(browRows * browRowH)
		th := h * float32(browRows) / float32(len(a.list))
		ty := float32(browListY) + (h-th)*float32(a.scroll)/float32(len(a.list)-browRows)
		c.Rect(browX+browW-4, ty, 2, th, cLine)
	}
	if len(a.list) == 0 {
		c.Text("Nothing here yet.", FBody, browX+browW/2, browListY+10, cDim, AlignC)
		c.Text("SAVE stores your own sounds.", FBody, browX+browW/2, browListY+30, cDim, AlignC)
	}
}

func (a *App) drawWidget(c Canvas, w *widget, slot int, picking bool) {
	m := &synth.Metas[w.pid]
	v := a.st.P[w.pid]
	val := m.Format(v)
	lc := cDim
	if slot >= 0 {
		lc = cAccent
	}
	if picking {
		lc = cYellow
	}
	label := m.Name
	if slot >= 0 && w.kind != wInline && w.kind != wSlider {
		label = fmt.Sprintf("%s·%d", m.Name, slot+1)
	}
	cx := w.x + w.w/2
	vc, sc := cText, cAccent2
	if a.inactive(w.pid) {
		vc, sc = cLine, cLine
	}
	switch w.kind {
	case wKnob:
		c.Text(label, FTiny, cx, w.y, lc, AlignC)
		a.drawKnob(c, cx, w.y+28, 13, m, v, cAccent)
		c.Text(val, FSmall, cx, w.y+46, vc, AlignC)
	case wStep:
		c.Text(label, FTiny, cx, w.y, lc, AlignC)
		c.Rect(w.x+3, w.y+17, w.w-6, 24, cWell)
		c.Text(clip(val, int(w.w/7)), FSmall, cx, w.y+23, sc, AlignC)
		c.Text("<  >", FTiny, cx, w.y+46, cLine, AlignC)
	case wToggle:
		c.Text(label, FTiny, cx, w.y, lc, AlignC)
		bg, tc := cWell, cDim
		if v >= 0.5 {
			bg, tc = cAccent2, cBG
		}
		c.Rect(w.x+3, w.y+17, w.w-6, 24, bg)
		c.Text(val, FSmall, cx, w.y+23, tc, AlignC)
	case wInline:
		c.Rect(w.x, w.y, w.w, w.h, cWell)
		tc := cAccent2
		if v == 0 && (strings.HasSuffix(m.Key, ".src") || strings.HasSuffix(m.Key, ".dst")) {
			tc = cLine
		}
		c.Text(clip(val, int(w.w/7.5)), FSmall, cx, w.y+w.h/2-6, tc, AlignC)
	case wSlider:
		ty := w.y + 6
		c.Rect(w.x, ty, w.w, 6, cWell)
		n := float32(m.ToNorm(v))
		mid := w.x + w.w/2
		px := w.x + w.w*n
		if px >= mid {
			c.Rect(mid, ty, px-mid, 6, cAccent)
		} else {
			c.Rect(px, ty, mid-px, 6, cAccent2)
		}
		c.Rect(mid-1, ty-2, 2, 10, cLine)
		c.Circle(px, ty+3, 6, cText)
		if slot >= 0 {
			c.Text(fmt.Sprintf("KNOB %d", slot+1), FTiny, w.x, w.y+20, cAccent, AlignL)
		}
		c.Text(val, FSmall, w.x+w.w, w.y+18, cText, AlignR)
	}
}

// inactive reports controls that currently do nothing (the LFO rate that isn't in use).
func (a *App) inactive(id synth.PID) bool {
	for _, b := range []synth.PID{synth.PL1Shape, synth.PL2Shape} {
		sync := a.st.P[b+1] >= 0.5
		if (id == b+2 && sync) || (id == b+3 && !sync) {
			return true
		}
	}
	return false
}

func (a *App) drawKnob(c Canvas, cx, cy, r float32, m *synth.Meta, v float64, col color.RGBA) {
	n := float32(m.ToNorm(v))
	c.Arc(cx, cy, r, knobA0, knobA0+knobSweep, 3, cWell)
	ang := knobA0 + knobSweep*n
	if m.Min < 0 && m.Kind == synth.KLin { // bipolar: draw from the centre
		mid := float32(knobA0 + knobSweep/2)
		if ang > mid+0.02 {
			c.Arc(cx, cy, r, mid, ang, 3, col)
		} else if ang < mid-0.02 {
			c.Arc(cx, cy, r, ang, mid, 3, cAccent2)
		}
	} else if n > 0.005 {
		c.Arc(cx, cy, r, knobA0, ang, 3, col)
	}
	c.Circle(cx, cy, r-4, cPanelHi)
	sx, sy := float32(math.Cos(float64(ang))), float32(math.Sin(float64(ang)))
	c.Polyline([]float32{cx + sx*(r-9), cy + sy*(r-9), cx + sx*(r-3), cy + sy*(r-3)}, 2.5, cText)
}

// drawScopes shows each oscillator's wave at its current (modulated) wavetable position.
func (a *App) drawScopes(c Canvas) {
	oscW := float32(mainW-8) / 2
	pts := make([]float32, 0, 2*96)
	for o := 0; o < 2; o++ {
		base := synth.PATable + synth.PID(9*o)
		x := float32(mainX) + float32(o)*(oscW+8) + 8
		y := float32(rowOscY + 22)
		const sh = 62
		c.Rect(x, y, scopeW, sh, cWell)
		tab := a.Eng.Table(int(a.st.P[base]))
		pos := a.st.PosA
		if o == 1 {
			pos = a.st.PosB
		}
		fp := pos * (synth.Frames - 1)
		i0 := int(fp)
		if i0 > synth.Frames-2 {
			i0 = synth.Frames - 2
		}
		fr := float32(fp - float64(i0))
		f0, f1 := tab.Frame(i0), tab.Frame(i0+1)
		pts = pts[:0]
		const n = 96
		for i := 0; i <= n; i++ {
			k := i * (synth.TableSize - 1) / n
			s := f0[k] + (f1[k]-f0[k])*fr
			if s > 1.4 {
				s = 1.4
			}
			if s < -1.4 {
				s = -1.4
			}
			pts = append(pts, x+4+float32(i)*(scopeW-8)/n, y+sh/2-s*(sh/2-5)/1.4)
		}
		col := cAccent
		if a.st.P[base+5] < 0.005 {
			col = cLine
		}
		c.Polyline(pts, 1.6, col)
		// position marker
		c.Rect(x, y+sh-3, scopeW, 3, cPanel)
		c.Rect(x+float32(pos)*(scopeW-6), y+sh-3, 6, 3, cAccent2)
	}
}

func (a *App) drawLFOs(c Canvas) {
	for k := 0; k < 2; k++ {
		for _, p := range a.panels {
			if p.title == fmt.Sprintf("LFO %d", k+1) {
				x := p.x + p.w - 62
				c.Rect(x, p.y+7, 54, 4, cWell)
				v := float32(a.st.LFO[k])
				c.Rect(x+27+v*24-3, p.y+6, 6, 6, cAccent2)
			}
		}
	}
}

func (a *App) drawHW(c Canvas, knobs [8]synth.PID, ccs [8]int, knobT [8]time.Time, learn, pickSlot, lastCC, lastCCv int) {
	c.Text("Right-click any control above, then turn a knob on your keyboard to put it there.  Click a CC label to re-learn one knob.", FTiny, mainX+170, hwY+5, cDim, AlignL)
	if lastCC >= 0 {
		c.Text(fmt.Sprintf("last moved: CC %d = %d", lastCC, lastCCv), FTiny, mainX+mainW-282, hwY+5, cAccent2, AlignR)
	}
	sw := float32(mainW-16) / 8
	for i := 0; i < 8; i++ {
		x := float32(mainX+8) + float32(i)*sw
		y := float32(hwY + 24)
		m := &synth.Metas[knobs[i]]
		v := a.st.P[knobs[i]]
		hot := time.Since(knobT[i]) < 400*time.Millisecond
		bg := cWell
		if hot {
			bg = dimmed(cAccent, 0.28)
		}
		c.Rect(x+2, y, sw-4, hwH-30, bg)
		a.drawKnob(c, x+32, y+31, 21, m, v, cAccent)
		c.Text(fmt.Sprintf("%d", i+1), FBig, x+32, y+21, cText, AlignC)
		// CC label (click = learn)
		ccTxt, ccCol := "turn a knob to map", cDim
		if ccs[i] >= 0 {
			ccTxt, ccCol = fmt.Sprintf("CC %d", ccs[i]), cAccent2
		}
		if learn == i {
			ccTxt, ccCol = "TURN A KNOB NOW", cYellow
		}
		c.Text(ccTxt, FTiny, x+62, y+9, ccCol, AlignL)
		c.Text(clip(m.Format(v), 10), FBig, x+62, y+24, cAccent, AlignL)
		// target (click = pick)
		tc := cText
		name := m.Long
		if name == "" {
			name = m.Name
		}
		if pickSlot == i {
			tc, name = cYellow, "CLICK A CONTROL..."
		}
		c.Rect(x+6, y+62, sw-12, 20, cPanelHi)
		c.Text(clip(name, 22), FSmall, x+sw/2, y+66, tc, AlignC)
	}
}

func (a *App) drawLoop(c Canvas) {
	l := a.st.Loop
	x := float32(mainX + 8 + 398)
	w := float32(mainX+mainW-96) - x - 150
	c.Rect(x, loopY+12, w, 16, cWell)
	bars := l.Bars
	if bars < 1 {
		bars = 1
	}
	for i := 1; i < bars*4; i++ {
		lc := cPanel
		hh := float32(6)
		if i%4 == 0 {
			lc, hh = cLine, 16
		}
		c.Rect(x+w*float32(i)/float32(bars*4), loopY+12, 1, hh, lc)
	}
	col := cGreen
	if l.State == 2 || l.Overdub {
		col = cRed
	}
	if l.State == 2 || l.State == 3 {
		c.Rect(x, loopY+12, w*float32(l.Pos), 16, dimmed(col, 0.45))
		c.Rect(x+w*float32(l.Pos)-1, loopY+9, 3, 22, col)
	}
	msg := "LOOPER: press LOOP REC, then play. It loops in time with the BPM."
	switch l.State {
	case 1:
		msg = "waiting for your first note"
	case 2:
		msg = "recording"
	case 3:
		msg = fmt.Sprintf("looping %d notes", l.Notes)
	case 4:
		msg = fmt.Sprintf("stopped, %d notes", l.Notes)
	}
	if l.State == 0 {
		c.Text(msg, FTiny, x+8, loopY+15, cDim, AlignL)
	} else {
		c.Text(msg, FSmall, x+w+10, loopY+13, cText, AlignL)
	}
}

func isBlack(m int) bool {
	switch m % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// keyRect returns the on-screen rectangle of a keyboard key.
func keyRect(m int) (x, y, w, h float32) {
	white := 0
	for n := kbLow; n < m; n++ {
		if !isBlack(n) {
			white++
		}
	}
	if isBlack(m) {
		return float32(kbX+white*kbWhiteW) - 7, kbY, 14, kbH * 0.6
	}
	return float32(kbX + white*kbWhiteW), kbY, kbWhiteW, kbH
}

func (a *App) drawKeyboard(c Canvas) {
	lit := func(m int) bool { return a.st.Notes[m] }
	for m := kbLow; m <= kbHigh; m++ {
		if isBlack(m) {
			continue
		}
		x, y, w, h := keyRect(m)
		col := color.RGBA{0xe6, 0xe2, 0xd8, 0xff}
		if lit(m) {
			col = cAccent
		}
		c.Rect(x+1, y, w-2, h, col)
		if m%12 == 0 {
			c.Text(noteName(m), FTiny, x+w/2, y+h-13, cPanel, AlignC)
		}
	}
	for m := kbLow; m <= kbHigh; m++ {
		if !isBlack(m) {
			continue
		}
		x, y, w, h := keyRect(m)
		col := cWell
		if lit(m) {
			col = dimmed(cAccent, 0.8)
		}
		c.Rect(x, y, w, h, col)
	}
	// where the computer keys play
	if a.kbBase >= kbLow && a.kbBase <= kbHigh {
		x0, _, _, _ := keyRect(a.kbBase)
		top := a.kbBase + 31
		if top > kbHigh {
			top = kbHigh
		}
		x1, _, w1, _ := keyRect(top)
		c.Rect(x0, kbY-4, x1+w1-x0, 3, cAccent2)
	}
}
