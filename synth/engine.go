package synth

import (
	"math"
	"sync"
	"time"
)

const (
	cb        = 16 // control block: modulation, envelopes and the sequencer run every 16 samples
	maxVoices = 12
	maxPoly   = 8
	maxUni    = 7
	ctlRate   = float64(SampleRate) / cb
)

const (
	envOff = iota
	envAtk
	envDec
	envRel
)

type env struct {
	v     float64
	stage int
}

type envCoef struct{ a, d, s, r float64 }

func mkEnv(a, d, s, r, rate float64) envCoef {
	return envCoef{a: 1 / (a * rate), d: math.Exp(-4.6 / (d * rate)), s: s, r: math.Exp(-4.6 / (r * rate))}
}

func (e *env) tick(c *envCoef) float64 {
	switch e.stage {
	case envAtk:
		e.v += c.a
		if e.v >= 1 {
			e.v, e.stage = 1, envDec
		}
	case envDec:
		e.v = c.s + (e.v-c.s)*c.d
	case envRel:
		e.v *= c.r
		if e.v < 0.0003 {
			e.v, e.stage = 0, envOff
		}
	}
	return e.v
}

// svf is a zero-delay state variable filter (Cytomic / Zavalishin form).
type svf struct{ ic1, ic2 float64 }

func (s *svf) tick(x, a1, a2, a3 float64) (bp, lp float64) {
	v3 := x - s.ic2
	bp = a1*s.ic1 + a2*v3
	lp = s.ic2 + a2*s.ic1 + a3*v3
	s.ic1 = 2*bp - s.ic1
	s.ic2 = 2*lp - s.ic2
	return
}

func tanhf(x float64) float64 {
	if x < -3 {
		return -1
	}
	if x > 3 {
		return 1
	}
	x2 := x * x
	return x * (27 + x2) / (27 + 9*x2)
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func mtof(m float64) float64 { return 440 * math.Pow(2, (m-69)/12) }

type voice struct {
	on, gate, sus, fresh bool
	note                 int
	vel, rnd             float64
	age                  int64
	pitch, target        float64
	phA, phB             [maxUni]float64
	phSub                float64
	eAmp, eFlt, eMod     env
	f                    [4]svf // L stage 1, L stage 2, R stage 1, R stage 2
	g                    float64
	am                   float64
}

type oscPrep struct {
	n      int
	inc    [maxUni]float64
	gl, gr [maxUni]float64
	t0, t1 []float32
	fr     float64
	level  float64
}

type lfoState struct {
	phase, val, sh, shNext, rateMod float64
}

type stackNote struct {
	note int
	vel  float64
}

// Engine is the whole synth. All exported methods are safe to call from any thread.
type Engine struct {
	mu      sync.Mutex
	P, S    Patch // target values and smoothed values
	stepped [NumParams]bool
	tabs    []*Table

	v         [maxVoices]voice
	ageCtr    int64
	newest    int
	stack     []stackNote
	refs      [128]int
	direct    [128]bool
	lastPitch float64
	havePitch bool
	pedal     bool
	bend      float64
	bendSm    float64
	wheel, at float64
	seed      uint32

	lfo     [2]lfoState
	distMod float64

	cAmp, cFlt, cMod envCoef
	glideCoef        float64

	bpm     float64
	beat    float64
	metro   bool
	clickPh float64
	clickEn float64
	clickHz float64
	lastBt  int64

	extOn    bool
	extLast  time.Time
	extIv    float64
	extTicks int64
	extBase  float64

	arp  arpState
	loop looper
	fx   fx
	rec  *recorder

	mixL, mixR, bufL, bufR, bMono [cb]float64
	outL, outR                    [cb]float64
	pend                          int
	readBuf                       []float32

	peak       float64
	dispPosA   float64
	dispPosB   float64
	dispCutoff float64
}

// NewEngine builds the wavetables and returns a synth playing the default patch.
func NewEngine() *Engine {
	e := &Engine{tabs: Tables(), bpm: 120, pend: cb, seed: 0x9e3779b9, extOn: true, lastBt: -1}
	for i := range Metas {
		e.stepped[i] = Metas[i].Stepped()
	}
	e.P = DefaultPatch()
	e.S = e.P
	e.fx.init()
	e.arp.init()
	e.loop.bars = 2
	return e
}

func (e *Engine) rand() float64 {
	e.seed ^= e.seed << 13
	e.seed ^= e.seed >> 17
	e.seed ^= e.seed << 5
	return float64(e.seed) / 4294967296.0
}

// ---------------------------------------------------------------- notes

func (e *Engine) startNote(note int, vel float64) {
	mode := int(e.P[PMode])
	glide := e.P[PGlide] > 0.002
	if mode == 0 {
		idx := e.pickVoice(note)
		v := &e.v[idx]
		wasOn := v.on
		e.initVoice(idx, note, vel)
		if glide && e.havePitch && !wasOn {
			v.pitch = e.lastPitch
		}
	} else {
		v := &e.v[0]
		overlap := len(e.stack) > 0 && v.on && v.gate
		e.pushStack(note, vel)
		if overlap && mode == 2 {
			v.note, v.target = note, float64(note)
			if !glide {
				v.pitch = v.target
			}
		} else {
			wasOn := v.on
			e.initVoice(0, note, vel)
			if glide && e.havePitch && mode == 1 && !wasOn {
				v.pitch = e.lastPitch
			}
			if mode == 2 && !overlap {
				v.pitch = v.target
			}
		}
		// Mono: silence anything left over from poly mode.
		for i := 1; i < maxVoices; i++ {
			if e.v[i].on && e.v[i].gate {
				e.releaseVoice(&e.v[i])
			}
		}
	}
	e.lastPitch, e.havePitch = float64(note), true
}

func (e *Engine) pushStack(note int, vel float64) {
	e.dropStack(note)
	e.stack = append(e.stack, stackNote{note, vel})
}

func (e *Engine) dropStack(note int) {
	for i := range e.stack {
		if e.stack[i].note == note {
			e.stack = append(e.stack[:i], e.stack[i+1:]...)
			return
		}
	}
}

func (e *Engine) pickVoice(note int) int {
	free, gated := -1, 0
	oldRel, oldAny := -1, -1
	for i := range e.v {
		v := &e.v[i]
		if !v.on {
			if free < 0 {
				free = i
			}
			continue
		}
		if v.note == note {
			return i
		}
		if v.gate {
			gated++
		} else if oldRel < 0 || v.age < e.v[oldRel].age {
			oldRel = i
		}
		if v.gate && (oldAny < 0 || v.age < e.v[oldAny].age) {
			oldAny = i
		}
	}
	if gated >= maxPoly && oldAny >= 0 {
		return oldAny
	}
	if free >= 0 {
		return free
	}
	if oldRel >= 0 {
		return oldRel
	}
	return oldAny
}

func (e *Engine) initVoice(idx, note int, vel float64) {
	v := &e.v[idx]
	if !v.on {
		ua, ub := int(e.P[PAUni]), int(e.P[PBUni])
		for j := 0; j < maxUni; j++ {
			v.phA[j], v.phB[j] = 0, 0
			if ua > 1 {
				v.phA[j] = e.rand()
			}
			if ub > 1 {
				v.phB[j] = e.rand()
			}
		}
		v.phSub = 0
		v.f = [4]svf{}
		v.eAmp.v, v.eFlt.v, v.eMod.v = 0, 0, 0
		v.fresh = true
		v.pitch = float64(note)
	}
	v.on, v.gate, v.sus = true, true, false
	v.note, v.vel, v.target = note, vel, float64(note)
	if e.P[PGlide] <= 0.002 {
		v.pitch = v.target
	}
	v.rnd = e.rand()
	v.eAmp.stage, v.eFlt.stage, v.eMod.stage = envAtk, envAtk, envAtk
	e.ageCtr++
	v.age = e.ageCtr
	e.newest = idx
	for k := 0; k < 2; k++ {
		if e.P[PL1Retrig+PID(5*k)] >= 0.5 {
			e.lfo[k].phase = 0
			e.lfo[k].sh, e.lfo[k].shNext = e.rand()*2-1, e.rand()*2-1
		}
	}
}

func (e *Engine) releaseVoice(v *voice) {
	v.gate, v.sus = false, false
	if v.eAmp.stage != envOff {
		v.eAmp.stage = envRel
	}
	v.eFlt.stage, v.eMod.stage = envRel, envRel
}

func (e *Engine) stopNote(note int) {
	if int(e.P[PMode]) == 0 {
		for i := range e.v {
			v := &e.v[i]
			if v.on && v.gate && v.note == note {
				if e.pedal {
					v.sus = true
				} else {
					e.releaseVoice(v)
				}
			}
		}
		e.dropStack(note)
		return
	}
	e.dropStack(note)
	v := &e.v[0]
	if len(e.stack) > 0 {
		top := e.stack[len(e.stack)-1]
		if v.on && v.gate && v.note != top.note {
			v.note, v.target = top.note, float64(top.note)
			if e.P[PGlide] <= 0.002 {
				v.pitch = v.target
			}
		}
		return
	}
	for i := range e.v {
		v := &e.v[i]
		if v.on && v.gate && (i == 0 || v.note == note) {
			if e.pedal {
				v.sus = true
			} else {
				e.releaseVoice(v)
			}
		}
	}
}

// trigger / releaseNote are the single path every sounding note takes (keys, arp and loop).
func (e *Engine) trigger(note int, vel float64, fromLoop bool) {
	if note < 0 || note > 127 {
		return
	}
	e.refs[note]++
	if !fromLoop {
		e.loop.record(e, note, vel, true)
	}
	e.startNote(note, vel)
}

func (e *Engine) releaseNote(note int, fromLoop bool) {
	if note < 0 || note > 127 {
		return
	}
	if e.refs[note] > 0 {
		e.refs[note]--
	}
	if !fromLoop {
		e.loop.record(e, note, 0, false)
	}
	if e.refs[note] == 0 {
		e.stopNote(note)
	}
}

// KeyDown plays a note from a keyboard (velocity 0..1). With the arp or note repeat on, the
// key joins the pattern instead of sounding directly.
func (e *Engine) KeyDown(note int, vel float64) {
	if note < 0 || note > 127 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if int(e.P[PArpMode]) != ArpOff {
		e.arp.press(e, note, vel)
		return
	}
	if e.direct[note] {
		e.releaseNote(note, false)
	}
	e.direct[note] = true
	e.trigger(note, vel, false)
}

// KeyUp releases a note started with KeyDown.
func (e *Engine) KeyUp(note int) {
	if note < 0 || note > 127 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.direct[note] {
		e.direct[note] = false
		e.releaseNote(note, false)
	}
	e.arp.release(note)
}

// SetPedal is the sustain pedal. With the arp on it acts as a latch.
func (e *Engine) SetPedal(down bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pedal = down
	if !down {
		if e.P[PArpLatch] < 0.5 {
			e.arp.unlatch()
		}
		for i := range e.v {
			if e.v[i].on && e.v[i].sus {
				e.releaseVoice(&e.v[i])
			}
		}
	}
}

// SetBend sets the pitch wheel, -1..1.
func (e *Engine) SetBend(v float64) { e.mu.Lock(); e.bend = clamp(v, -1, 1); e.mu.Unlock() }

// SetWheel sets the mod wheel, 0..1.
func (e *Engine) SetWheel(v float64) { e.mu.Lock(); e.wheel = clamp(v, 0, 1); e.mu.Unlock() }

// SetAftertouch sets channel pressure, 0..1.
func (e *Engine) SetAftertouch(v float64) { e.mu.Lock(); e.at = clamp(v, 0, 1); e.mu.Unlock() }

// Panic stops every note, the arp and loop playback.
func (e *Engine) Panic() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.arp.clear()
	e.loop.silence(e)
	if e.loop.state == loopPlaying || e.loop.state == loopRecording || e.loop.state == loopArmed {
		e.loop.stop(e)
	}
	e.stack = e.stack[:0]
	e.refs = [128]int{}
	e.direct = [128]bool{}
	e.pedal = false
	for i := range e.v {
		if e.v[i].on {
			e.releaseVoice(&e.v[i])
		}
	}
}

// ---------------------------------------------------------------- parameters

// Set changes one parameter (clamped to its range).
func (e *Engine) Set(id PID, v float64) {
	e.mu.Lock()
	e.setLocked(id, v)
	e.mu.Unlock()
}

func (e *Engine) setLocked(id PID, v float64) {
	v = Metas[id].Clamp(v)
	if id == PArpMode && int(v) == ArpOff && int(e.P[id]) != ArpOff {
		e.arp.flush(e)
		e.arp.clear()
	}
	if id == PArpLatch && v < 0.5 {
		e.arp.unlatch()
	}
	e.P[id] = v
}

// SetNorm sets a parameter from a 0..1 knob position.
func (e *Engine) SetNorm(id PID, n float64) { e.Set(id, Metas[id].FromNorm(n)) }

// Get returns a parameter's current value.
func (e *Engine) Get(id PID) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.P[id]
}

// LoadPatch switches to a new sound. Performance settings (arp) are kept.
func (e *Engine) LoadPatch(p *Patch) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range Metas {
		if Metas[i].Global {
			continue
		}
		e.P[i] = Metas[i].Clamp(p[i])
		e.S[i] = e.P[i]
	}
}

// Patch returns a copy of the current sound.
func (e *Engine) Patch() Patch {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.P
}

// ---------------------------------------------------------------- audio

// Read implements io.Reader for the audio player: 32-bit float, stereo, little endian.
func (e *Engine) Read(b []byte) (int, error) {
	frames := len(b) / 8
	if frames == 0 {
		return 0, nil
	}
	if cap(e.readBuf) < frames*2 {
		e.readBuf = make([]float32, frames*2)
	}
	tmp := e.readBuf[:frames*2]
	e.Process(tmp)
	for i, f := range tmp {
		u := math.Float32bits(f)
		b[4*i] = byte(u)
		b[4*i+1] = byte(u >> 8)
		b[4*i+2] = byte(u >> 16)
		b[4*i+3] = byte(u >> 24)
	}
	return frames * 8, nil
}

// Process renders interleaved stereo samples.
func (e *Engine) Process(out []float32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(out) / 2
	for i := 0; i < n; {
		if e.pend >= cb {
			e.block()
			e.pend = 0
		}
		for e.pend < cb && i < n {
			out[2*i] = float32(e.outL[e.pend])
			out[2*i+1] = float32(e.outR[e.pend])
			e.pend++
			i++
		}
	}
}

func (e *Engine) block() {
	S := &e.S
	for i := range S {
		if e.stepped[i] {
			S[i] = e.P[i]
		} else {
			S[i] += (e.P[i] - S[i]) * 0.15
		}
	}
	e.bendSm += (e.bend - e.bendSm) * 0.2

	// Transport and sequencers.
	e.beat += e.bpm / 60 * cb / SampleRate
	e.loop.tick(e)
	e.arp.tick(e)

	// Envelope and glide coefficients.
	e.cAmp = mkEnv(S[PAmpA], S[PAmpD], S[PAmpS], S[PAmpR], SampleRate)
	e.cFlt = mkEnv(S[PFltA], S[PFltD], S[PFltS], S[PFltR], ctlRate)
	e.cMod = mkEnv(S[PModA], S[PModD], S[PModS], S[PModR], ctlRate)
	e.glideCoef = 1
	if S[PGlide] > 0.002 {
		e.glideCoef = 1 - math.Exp(-cb/(SampleRate*S[PGlide]/3))
	}

	// LFOs.
	for k := 0; k < 2; k++ {
		base := PL1Shape + PID(5*k)
		l := &e.lfo[k]
		sync := S[base+1] >= 0.5
		beats := LFODivs[int(S[base+3])].Beats
		rate := S[base+2]
		if sync {
			rate = e.bpm / 60 / beats
		}
		prev := l.phase
		if sync && S[base+4] < 0.5 && l.rateMod == 0 {
			l.phase = frac(e.beat / beats) // locked to the transport
		} else {
			l.phase = frac(l.phase + rate*math.Pow(2, 3*l.rateMod)*cb/SampleRate)
		}
		if l.phase < prev {
			l.sh, l.shNext = l.shNext, e.rand()*2-1
		}
		p := l.phase
		switch int(S[base]) {
		case 0:
			l.val = math.Sin(2 * math.Pi * p)
		case 1:
			if p < 0.5 {
				l.val = 4*p - 1
			} else {
				l.val = 3 - 4*p
			}
		case 2:
			l.val = 1 - 2*p
		case 3:
			l.val = 2*p - 1
		case 4:
			if p < 0.5 {
				l.val = 1
			} else {
				l.val = -1
			}
		case 5:
			l.val = l.sh
		default:
			l.val = lerp(l.sh, l.shNext, 0.5-0.5*math.Cos(math.Pi*p))
		}
	}

	// Modulation of global destinations (LFO rates, distortion) uses the newest voice.
	nv := &e.v[e.newest]
	gsrc := e.sources(nv)
	e.lfo[0].rateMod, e.lfo[1].rateMod, e.distMod = 0, 0, 0
	for s := 0; s < ModSlots; s++ {
		b := PM1Src + PID(3*s)
		si, di := int(S[b]), int(S[b+1])
		if si == 0 {
			continue
		}
		switch di {
		case DL1Rate:
			e.lfo[0].rateMod += S[b+2] * gsrc[si]
		case DL2Rate:
			e.lfo[1].rateMod += S[b+2] * gsrc[si]
		case DDist:
			e.distMod += S[b+2] * gsrc[si]
		}
	}

	for i := range e.mixL {
		e.mixL[i], e.mixR[i] = 0, 0
	}
	for i := range e.v {
		if e.v[i].on {
			e.renderVoice(&e.v[i], i == e.newest)
		}
	}
	e.fx.process(e)

	// Recorder, meter, metronome.
	var pk float64
	for i := 0; i < cb; i++ {
		pk = math.Max(pk, math.Max(math.Abs(e.outL[i]), math.Abs(e.outR[i])))
	}
	e.peak = math.Max(pk, e.peak*0.995)
	if e.rec != nil {
		e.rec.add(e.outL[:], e.outR[:])
	}
	e.click()
}

func (e *Engine) sources(v *voice) [NumSrc]float64 {
	var s [NumSrc]float64
	s[SrcLFO1], s[SrcLFO2] = e.lfo[0].val, e.lfo[1].val
	s[SrcWheel], s[SrcAT] = e.wheel, e.at
	s[SrcL1Wheel] = e.lfo[0].val * e.wheel
	if v.on {
		s[SrcModEnv], s[SrcFltEnv], s[SrcAmpEnv] = v.eMod.v, v.eFlt.v, v.eAmp.v
		s[SrcVel] = v.vel
		s[SrcKey] = (v.pitch - 36) / 36
		s[SrcRand] = v.rnd*2 - 1
	}
	return s
}

func (e *Engine) prepOsc(o *oscPrep, base PID, pitch, posMod, detMod, lvlMod float64) {
	S := &e.S
	n := int(S[base+6])
	if n < 1 {
		n = 1
	}
	if n > maxUni {
		n = maxUni
	}
	o.n = n
	o.level = clamp(S[base+5]+lvlMod, 0, 1.5)
	f := mtof(pitch + 12*S[base+2] + S[base+3] + S[base+4]/100)
	f = clamp(f, 4, 20000)
	tab := e.tabs[int(S[base])]
	fp := clamp(S[base+1]+posMod, 0, 1) * (Frames - 1)
	i0 := int(fp)
	if i0 > Frames-2 {
		i0 = Frames - 2
	}
	o.fr = fp - float64(i0)
	if n == 1 {
		k := mipFor(f)
		o.t0, o.t1 = tab.mip[k][i0], tab.mip[k][i0+1]
		o.inc[0] = f / SampleRate
		o.gl[0], o.gr[0] = 1, 1
		return
	}
	det := clamp(S[base+7]+detMod, 0, 1)
	cents := 60 * det * det
	k := mipFor(f * math.Pow(2, cents/1200))
	o.t0, o.t1 = tab.mip[k][i0], tab.mip[k][i0+1]
	spread := S[base+8]
	norm := 1 / math.Sqrt(float64(n))
	for j := 0; j < n; j++ {
		p := 2*float64(j)/float64(n-1) - 1
		o.inc[j] = f * math.Pow(2, cents*p/1200) / SampleRate
		pp := p
		if j%2 == 1 {
			pp = -p
		}
		a := (pp*spread + 1) * math.Pi / 4
		o.gl[j] = math.Sqrt2 * math.Cos(a) * norm
		o.gr[j] = math.Sqrt2 * math.Sin(a) * norm
	}
}

// oscBlock adds one oscillator (all unison voices) into L/R. tap, if not nil, receives the
// first unison voice before level (the FM source). fm, if not nil, phase-modulates it.
func oscBlock(o *oscPrep, ph *[maxUni]float64, L, R *[cb]float64, stereo bool, tap, fm *[cb]float64, fmDepth float64) {
	t0, t1 := o.t0, o.t1
	fr := o.fr
	for j := 0; j < o.n; j++ {
		p := ph[j]
		inc := o.inc[j]
		gl, gr := o.gl[j]*o.level, o.gr[j]*o.level
		for i := 0; i < cb; i++ {
			q := p
			if fm != nil {
				q += fmDepth * fm[i]
				q -= math.Floor(q)
			}
			x := q * TableSize
			idx := int(x)
			f := x - float64(idx)
			idx &= TableSize - 1
			a0, a1 := float64(t0[idx]), float64(t0[idx+1])
			b0, b1 := float64(t1[idx]), float64(t1[idx+1])
			s0 := a0 + (a1-a0)*f
			s := s0 + (b0+(b1-b0)*f-s0)*fr
			if j == 0 && tap != nil {
				tap[i] = s
			}
			L[i] += s * gl
			if stereo {
				R[i] += s * gr
			}
			p += inc
			if p >= 1 {
				p -= 1
			}
		}
		ph[j] = p
	}
}

func polyBlep(t, dt float64) float64 {
	if t < dt {
		t /= dt
		return t + t - t*t - 1
	}
	if t > 1-dt {
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

func (e *Engine) renderVoice(v *voice, newest bool) {
	S := &e.S
	v.eFlt.tick(&e.cFlt)
	v.eMod.tick(&e.cMod)
	src := e.sources(v)
	var md [NumDst]float64
	for s := 0; s < ModSlots; s++ {
		b := PM1Src + PID(3*s)
		si, di := int(S[b]), int(S[b+1])
		if si == 0 || di == 0 {
			continue
		}
		a := S[b+2]
		if di == DPitch || di == DPitchA || di == DPitchB {
			a = 36 * a * math.Abs(a)
		}
		md[di] += a * src[si]
	}

	v.pitch += (v.target - v.pitch) * e.glideCoef
	base := v.pitch + e.bendSm*S[PBend] + md[DPitch]

	var oa, ob oscPrep
	e.prepOsc(&oa, PATable, base+md[DPitchA], md[DPosA], md[DDetune], md[DLevelA])
	e.prepOsc(&ob, PBTable, base+md[DPitchB], md[DPosB], md[DDetune], md[DLevelB])
	if newest {
		e.dispPosA = clamp(S[PAPos]+md[DPosA], 0, 1)
		e.dispPosB = clamp(S[PBPos]+md[DPosB], 0, 1)
	}
	stereo := (oa.n > 1 && S[PASpread] > 0.01 && oa.level > 0) || (ob.n > 1 && S[PBSpread] > 0.01 && ob.level > 0) || md[DPan] != 0
	L, R := &e.bufL, &e.bufR
	for i := range L {
		L[i], R[i] = 0, 0
	}
	fm := clamp(S[PFM]+md[DFM], 0, 1)
	fm = fm * fm * 0.8
	if ob.level > 0 || fm > 0 {
		oscBlock(&ob, &v.phB, L, R, stereo, &e.bMono, nil, 0)
	}
	if oa.level > 0 {
		if fm > 0 {
			oscBlock(&oa, &v.phA, L, R, stereo, nil, &e.bMono, fm)
		} else {
			oscBlock(&oa, &v.phA, L, R, stereo, nil, nil, 0)
		}
	}

	// Sub oscillator and noise (always centred).
	subLvl := clamp(S[PSubLevel]+md[DSub], 0, 1.5)
	nzLvl := clamp(S[PNoise]+md[DNoise], 0, 1.5)
	if subLvl > 0 || nzLvl > 0 {
		dt := clamp(mtof(base+12*S[PSubOct]), 4, 8000) / SampleRate
		shape := int(S[PSubShape])
		p := v.phSub
		for i := 0; i < cb; i++ {
			var s float64
			if subLvl > 0 {
				switch shape {
				case 0:
					s = math.Sin(2 * math.Pi * p)
				case 1:
					s = 1 - 4*math.Abs(frac(p+0.25)-0.5)
				case 2:
					if p < 0.5 {
						s = 1
					} else {
						s = -1
					}
					s += polyBlep(p, dt) - polyBlep(frac(p+0.5), dt)
					s *= 0.7
				default:
					s = (2*p - 1 - polyBlep(p, dt)) * 0.8
				}
				s *= subLvl * 0.8
			}
			if nzLvl > 0 {
				s += (e.rand()*2 - 1) * nzLvl * 0.5
			}
			L[i] += s
			if stereo {
				R[i] += s
			}
			p += dt
			if p >= 1 {
				p -= 1
			}
		}
		v.phSub = p
	}

	// Filter coefficients.
	oct := S[PFEnv]*v.eFlt.v*8 + md[DCutoff]*6 + S[PFKey]*(v.pitch-48)/12
	fc := clamp(S[PCutoff]*math.Pow(2, oct), 16, 20000)
	if newest {
		e.dispCutoff = fc
	}
	gNew := math.Tan(math.Pi * fc / SampleRate)
	if v.fresh {
		v.g = gNew
	}
	dg := (gNew - v.g) / cb
	g := v.g
	v.g = gNew
	res := clamp(S[PReso]+md[DReso], 0, 1)
	ftype := int(S[PFType])
	k := 2 - 1.96*res
	k2 := 1.414 - 0.9*res
	if ftype == 5 {
		k = 2 - 1.985*res
		k2 = k
	}
	drv := clamp(S[PFDrive]+md[DFDrive], 0, 1)
	dg1 := 1 + drv*drv*24
	dcomp := 1 / math.Pow(dg1, 0.3)

	// Amp.
	vg := 1 - S[PVelSens] + S[PVelSens]*v.vel
	amNew := clamp(1+md[DAmp], 0, 2) * vg
	if v.fresh {
		v.am = amNew
	}
	dam := (amNew - v.am) / cb
	am := v.am
	v.am = amNew
	v.fresh = false
	pan := clamp(md[DPan], -1, 1)
	pa := (pan + 1) * math.Pi / 4
	pl, pr := math.Sqrt2*math.Cos(pa), math.Sqrt2*math.Sin(pa)

	chans := 1
	if stereo {
		chans = 2
	}
	for i := 0; i < cb; i++ {
		g += dg
		am += dam
		a1 := 1 / (1 + g*(g+k))
		a2 := g * a1
		a3 := g * a2
		b1 := 1 / (1 + g*(g+k2))
		b2 := g * b1
		b3 := g * b2
		ea := v.eAmp.tick(&e.cAmp) * am
		var y [2]float64
		for c := 0; c < chans; c++ {
			x := L[i]
			if c == 1 {
				x = R[i]
			}
			if drv > 0.001 {
				x = tanhf(x*dg1) * dcomp
			}
			s1, s2 := &v.f[2*c], &v.f[2*c+1]
			bp, lp := s1.tick(x, a1, a2, a3)
			var o float64
			switch ftype {
			case 0:
				_, o = s2.tick(lp, b1, b2, b3)
			case 1:
				o = lp
			case 2:
				o = bp * (0.6 + 0.4*k)
			case 3:
				o = x - k*bp - lp
			case 4:
				o = x - k*bp
			default:
				_, o = s2.tick(tanhf(lp*1.4)*0.75, a1, a2, a3)
			}
			y[c] = o * ea
		}
		if stereo {
			e.mixL[i] += y[0] * pl
			e.mixR[i] += y[1] * pr
		} else {
			e.mixL[i] += y[0]
			e.mixR[i] += y[0]
		}
	}
	if v.f[0].ic1 != v.f[0].ic1 || v.f[2].ic1 != v.f[2].ic1 { // NaN guard
		v.f = [4]svf{}
	}
	if v.eAmp.stage == envOff {
		v.on, v.gate = false, false
	}
}

// ---------------------------------------------------------------- transport

// SetBPM sets the tempo (30..300).
func (e *Engine) SetBPM(b float64) {
	e.mu.Lock()
	e.bpm = clamp(b, 30, 300)
	e.mu.Unlock()
}

// SetMetronome turns the click on or off.
func (e *Engine) SetMetronome(on bool) { e.mu.Lock(); e.metro = on; e.mu.Unlock() }

// SetExtSync chooses whether incoming MIDI clock sets the tempo.
func (e *Engine) SetExtSync(on bool) { e.mu.Lock(); e.extOn = on; e.mu.Unlock() }

// MIDIClock is called for every MIDI timing clock message (24 per quarter note).
func (e *Engine) MIDIClock(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	iv := now.Sub(e.extLast).Seconds()
	e.extLast = now
	if iv <= 0 || iv > 0.25 { // first tick, or the clock restarted
		e.extIv = 0
		e.extTicks = 0
		e.extBase = e.beat
		return
	}
	if e.extIv == 0 {
		e.extIv = iv
	} else {
		e.extIv += (iv - e.extIv) * 0.08
	}
	e.extTicks++
	if !e.extOn {
		return
	}
	if e.extTicks > 12 {
		e.bpm = clamp(60/(24*e.extIv), 30, 300)
	}
	target := e.extBase + float64(e.extTicks)/24
	if d := target - e.beat; math.Abs(d) < 0.5 {
		e.beat += d * 0.1
	} else {
		e.extBase = e.beat - float64(e.extTicks)/24
	}
}

// MIDIStart restarts the bar (MIDI Start message).
func (e *Engine) MIDIStart() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.extOn {
		return
	}
	e.resetTransport()
	e.extTicks, e.extBase = 0, 0
}

func (e *Engine) resetTransport() {
	e.beat = 0
	e.lastBt = -1
	e.arp.running = false
	if e.loop.state == loopPlaying {
		e.loop.silence(e)
		e.loop.start, e.loop.pos = 0, 0
	}
}

func (e *Engine) extActive() bool {
	return e.extOn && e.extTicks > 12 && time.Since(e.extLast) < 500*time.Millisecond
}

// gridLocked reports whether sequenced notes should snap to the transport's beat grid.
func (e *Engine) gridLocked() bool {
	return e.metro || e.extActive() || e.loop.state == loopPlaying || e.loop.state == loopRecording
}

func (e *Engine) click() {
	bt := int64(math.Floor(e.beat))
	if bt != e.lastBt {
		if e.lastBt >= 0 && e.metro {
			e.clickEn, e.clickPh = 1, 0
			e.clickHz = 1400
			if bt%4 == 0 {
				e.clickHz = 2100
			}
		}
		e.lastBt = bt
	}
	if e.clickEn < 0.001 {
		return
	}
	for i := 0; i < cb; i++ {
		s := math.Sin(2*math.Pi*e.clickPh) * e.clickEn * 0.25
		e.clickPh += e.clickHz / SampleRate
		e.clickEn *= 0.9985
		e.outL[i] += s
		e.outR[i] += s
	}
}

// ---------------------------------------------------------------- status for the UI

// Status is a snapshot of the engine for drawing.
type Status struct {
	P           Patch
	BPM, Beat   float64
	Ext         bool
	Metro       bool
	Voices      int
	PosA, PosB  float64
	Cutoff      float64
	LFO         [2]float64
	Notes       [128]bool
	Peak        float64
	Loop        LoopStatus
	Recording   bool
	RecSeconds  float64
	ArpHeld     int
	Wheel, Bend float64
}

// Snapshot copies everything the UI shows.
func (e *Engine) Snapshot() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Status{P: e.P, BPM: e.bpm, Beat: e.beat, Ext: e.extActive(), Metro: e.metro,
		PosA: e.dispPosA, PosB: e.dispPosB, Cutoff: e.dispCutoff, Peak: e.peak,
		LFO: [2]float64{e.lfo[0].val, e.lfo[1].val}, Wheel: e.wheel, Bend: e.bend}
	any := false
	for i := range e.v {
		if e.v[i].on {
			st.Voices++
			if e.v[i].gate {
				st.Notes[e.v[i].note] = true
				any = true
			}
		}
	}
	if !any {
		st.PosA, st.PosB = e.P[PAPos], e.P[PBPos]
	}
	st.Loop = e.loop.status(e)
	st.ArpHeld = len(e.arp.pool(e))
	if e.rec != nil {
		st.Recording = true
		st.RecSeconds = float64(e.rec.frames) / SampleRate
	}
	return st
}

// Table returns a wavetable for display.
func (e *Engine) Table(i int) *Table {
	if i < 0 || i >= len(e.tabs) {
		i = 0
	}
	return e.tabs[i]
}
