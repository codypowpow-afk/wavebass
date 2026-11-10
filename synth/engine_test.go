package synth

import (
	"math"
	"testing"
	"time"
)

// render returns the peak and the loudest 50 ms RMS window.
func render(e *Engine, sec float64) (peak, rms float64) {
	buf := make([]float32, 2*int(sec*SampleRate))
	e.Process(buf)
	const w = 4800
	for i := 0; i+w <= len(buf); i += w {
		var sum float64
		for _, s := range buf[i : i+w] {
			v := float64(s)
			if v != v {
				return math.NaN(), math.NaN()
			}
			sum += v * v
			peak = math.Max(peak, math.Abs(v))
		}
		rms = math.Max(rms, math.Sqrt(sum/w))
	}
	return peak, rms
}

func TestPresets(t *testing.T) {
	e := NewEngine()
	seen := map[string]bool{}
	for i := range Presets {
		p := &Presets[i]
		if seen[p.Name] {
			t.Errorf("duplicate preset name %q", p.Name)
		}
		seen[p.Name] = true
		ok := false
		for _, c := range Categories {
			ok = ok || c == p.Cat
		}
		if !ok {
			t.Errorf("%s: unknown category %q", p.Name, p.Cat)
		}
		e.Panic()
		render(e, 0.6)
		e.LoadPatch(&p.Patch)
		note := 33
		e.KeyDown(note, 0.8)
		if int(p.Patch[PMode]) == poly {
			e.KeyDown(note+7, 0.8)
			e.KeyDown(note+12, 0.8)
		}
		pk, rms := render(e, 1.0)
		e.KeyUp(note)
		e.KeyUp(note + 7)
		e.KeyUp(note + 12)
		render(e, 0.3)
		t.Logf("%-16s %-14s peak %.2f  rms %.3f (%.1f dB)", p.Name, p.Cat, pk, rms, 20*math.Log10(rms+1e-9))
		if pk != pk {
			t.Errorf("%s: NaN", p.Name)
		}
		if rms < 0.15 || rms > 0.45 {
			t.Errorf("%s: level off (rms %.4f)", p.Name, rms)
		}
		if pk > 1.0001 {
			t.Errorf("%s: clips (%.2f)", p.Name, pk)
		}
	}
	t.Logf("%d presets", len(Presets))
}

func TestCPU(t *testing.T) {
	e := NewEngine()
	for _, name := range []string{"Hoover Chord", "Neuro Reese", "Pure Sub"} {
		for i := range Presets {
			if Presets[i].Name != name {
				continue
			}
			e.Panic()
			e.LoadPatch(&Presets[i].Patch)
			for n := 0; n < 8; n++ {
				e.KeyDown(30+n*3, 0.8)
			}
			t0 := time.Now()
			render(e, 4)
			t.Logf("%s: %.1f%% of one core (8 keys)", name, time.Since(t0).Seconds()/4*100)
			for n := 0; n < 8; n++ {
				e.KeyUp(30 + n*3)
			}
		}
	}
}

func countOn(e *Engine) int {
	n := 0
	for i := range e.v {
		if e.v[i].on && e.v[i].gate {
			n++
		}
	}
	return n
}

func TestArpAndRepeat(t *testing.T) {
	e := NewEngine()
	e.SetBPM(120)
	e.Set(PMode, poly)
	e.Set(PArpMode, ArpRepeat)
	e.Set(PArpRate, 3) // 1/16 = 8 per second at 120
	starts := 0
	last := int64(0)
	e.KeyDown(36, 0.9)
	buf := make([]float32, 2*cb)
	for i := 0; i < SampleRate/cb; i++ {
		e.Process(buf)
		if a := e.v[e.newest].age; a != last {
			last = a
			starts++
		}
	}
	if starts < 7 || starts > 9 {
		t.Errorf("note repeat: %d triggers in one second, want 8", starts)
	}
	e.KeyUp(36)
	render(e, 0.5)
	if n := countOn(e); n != 0 {
		t.Errorf("%d notes stuck after note repeat", n)
	}
	for i, r := range e.refs {
		if r != 0 {
			t.Errorf("ref leak on note %d", i)
		}
	}
	// Arp up over a chord, then switch the arp off while holding.
	e.Set(PArpMode, ArpUp)
	e.KeyDown(36, 0.9)
	e.KeyDown(40, 0.9)
	e.KeyDown(43, 0.9)
	render(e, 1)
	e.Set(PArpMode, ArpOff)
	e.KeyUp(36)
	e.KeyUp(40)
	e.KeyUp(43)
	render(e, 0.5)
	if n := countOn(e); n != 0 {
		t.Errorf("%d notes stuck after arp off", n)
	}
}

func TestLooper(t *testing.T) {
	e := NewEngine()
	e.SetBPM(240) // 1 bar = 1 second
	e.Set(PMode, poly)
	e.SetLoopBars(1)
	e.LoopRec()
	render(e, 0.33)
	e.KeyDown(36, 0.9)
	render(e, 0.25)
	e.KeyUp(36)
	render(e, 0.25)
	e.KeyDown(43, 0.9)
	render(e, 0.2)
	e.KeyUp(43)
	render(e, 0.4) // past the end of the bar: now looping
	st := e.Snapshot()
	if st.Loop.State != loopPlaying || st.Loop.Notes != 2 {
		t.Fatalf("loop state %d notes %d", st.Loop.State, st.Loop.Notes)
	}
	// Count loop-triggered notes over 3 more bars.
	starts, last := 0, e.ageCtr
	buf := make([]float32, 2*cb)
	for i := 0; i < 3*SampleRate/cb; i++ {
		e.Process(buf)
		if e.ageCtr != last {
			starts += int(e.ageCtr - last)
			last = e.ageCtr
		}
	}
	if starts != 6 {
		t.Errorf("loop played %d notes in 3 bars, want 6", starts)
	}
	e.LoopPlay()
	render(e, 0.5)
	if n := countOn(e); n != 0 {
		t.Errorf("%d notes stuck after loop stop", n)
	}
	e.LoopClear()
	if e.Snapshot().Loop.State != loopEmpty {
		t.Error("loop not cleared")
	}
}

func TestMonoLegato(t *testing.T) {
	e := NewEngine()
	e.Set(PMode, mono)
	e.KeyDown(36, 0.8)
	render(e, 0.1)
	e.KeyDown(40, 0.8)
	render(e, 0.1)
	e.KeyUp(40)
	render(e, 0.1)
	if !e.v[0].gate || e.v[0].note != 36 {
		t.Errorf("mono should fall back to the held note: gate %v note %d", e.v[0].gate, e.v[0].note)
	}
	e.KeyUp(36)
	render(e, 0.3)
	if countOn(e) != 0 {
		t.Error("stuck note")
	}
}

func TestParamRoundTrip(t *testing.T) {
	for i := range Metas {
		m := &Metas[i]
		for _, n := range []float64{0, 0.25, 0.5, 1} {
			v := m.FromNorm(n)
			if v < m.Min-1e-9 || v > m.Max+1e-9 {
				t.Errorf("%s: %v out of range", m.Key, v)
			}
			if !m.Stepped() && math.Abs(m.ToNorm(v)-n) > 1e-6 {
				t.Errorf("%s: round trip %v -> %v", m.Key, n, m.ToNorm(v))
			}
		}
	}
}

func TestPitch(t *testing.T) {
	e := NewEngine()
	e.LoadPatch(&Presets[0].Patch) // Pure Sub
	e.KeyDown(33, 0.8)             // A1 = 55 Hz
	buf := make([]float32, 2*SampleRate)
	e.Process(buf)
	cross := 0
	for i := 2; i < len(buf); i += 2 {
		if buf[i-2] < 0 && buf[i] >= 0 {
			cross++
		}
	}
	if cross < 54 || cross > 56 {
		t.Errorf("A1 measured %d Hz, want 55", cross)
	}
}
