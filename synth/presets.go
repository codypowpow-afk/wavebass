package synth

// Preset is a named sound plus what the eight hardware knobs control for it.
type Preset struct {
	Name  string
	Cat   string
	Patch Patch
	Knobs [8]PID
	User  bool
}

// Categories in display order.
var Categories = []string{"Sub", "Classic", "Acid", "FM / Digital", "Wobble", "Growl", "Neuro / DnB", "Weird", "Poly / Stab", "User"}

type kv map[PID]float64

// Standard knob layouts.
var (
	KnobsStd = [8]PID{PCutoff, PReso, PAPos, PFEnv, PFltD, PFDrive, PDistMix, PSubLevel}
	kWob     = [8]PID{PCutoff, PReso, PL1Div, PM1Amt, PAPos, PL1Shape, PDistMix, PSubLevel}
	kGrowl   = [8]PID{PCutoff, PAPos, PL1Div, PM1Amt, PFM, PReso, PDistDrive, PDistMix}
	k808     = [8]PID{PAmpD, PModD, PM1Amt, PAPos, PCutoff, PGlide, PDistMix, PDistDrive}
	kFM      = [8]PID{PAPos, PFM, PM1Amt, PModD, PCutoff, PReso, PDistMix, PSubLevel}
	kAcid    = [8]PID{PCutoff, PReso, PFEnv, PFltD, PM1Amt, PGlide, PDistMix, PDistDrive}
	kReese   = [8]PID{PCutoff, PReso, PADetune, PBFine, PM1Amt, PL1Rate, PDistMix, PChorus}
	kSpace   = [8]PID{PCutoff, PReso, PAPos, PM1Amt, PDlyMix, PDlyFB, PRevMix, PCrush}
)

func lfoDiv(name string) float64 {
	for i, d := range LFODivs {
		if d.Name == name {
			return float64(i)
		}
	}
	panic("unknown division " + name)
}

func oa(tab int, pos, lvl float64, oct int) kv {
	return kv{PATable: float64(tab), PAPos: pos, PALevel: lvl, PAOct: float64(oct)}
}
func ob(tab int, pos, lvl float64, oct int) kv {
	return kv{PBTable: float64(tab), PBPos: pos, PBLevel: lvl, PBOct: float64(oct)}
}
func uniA(n int, det, spr float64) kv { return kv{PAUni: float64(n), PADetune: det, PASpread: spr} }
func uniB(n int, det, spr float64) kv { return kv{PBUni: float64(n), PBDetune: det, PBSpread: spr} }
func tuneA(semi int, fine float64) kv { return kv{PASemi: float64(semi), PAFine: fine} }
func tuneB(semi int, fine float64) kv { return kv{PBSemi: float64(semi), PBFine: fine} }
func sub(shape, oct int, lvl float64) kv {
	return kv{PSubShape: float64(shape), PSubOct: float64(oct), PSubLevel: lvl}
}
func flt(typ int, cut, res, env, key float64) kv {
	return kv{PFType: float64(typ), PCutoff: cut, PReso: res, PFEnv: env, PFKey: key}
}
func aenv(a, d, s, r float64) kv { return kv{PAmpA: a, PAmpD: d, PAmpS: s, PAmpR: r} }
func fenv(a, d, s, r float64) kv { return kv{PFltA: a, PFltD: d, PFltS: s, PFltR: r} }
func menv(a, d, s, r float64) kv { return kv{PModA: a, PModD: d, PModS: s, PModR: r} }

// lfo1 / lfo2: tempo-synced, retriggered on each note.
func lfo1(shape int, div string) kv {
	return kv{PL1Shape: float64(shape), PL1Sync: 1, PL1Div: lfoDiv(div), PL1Retrig: 1}
}
func lfo2(shape int, div string) kv {
	return kv{PL2Shape: float64(shape), PL2Sync: 1, PL2Div: lfoDiv(div), PL2Retrig: 1}
}

// lfo1hz / lfo2hz: free running.
func lfo1hz(shape int, hz float64) kv { return kv{PL1Shape: float64(shape), PL1Sync: 0, PL1Rate: hz} }
func lfo2hz(shape int, hz float64) kv { return kv{PL2Shape: float64(shape), PL2Sync: 0, PL2Rate: hz} }

func mod(slot, src, dst int, amt float64) kv {
	b := PM1Src + PID(3*(slot-1))
	return kv{b: float64(src), b + 1: float64(dst), b + 2: amt}
}
func dist(typ int, drive, mix float64) kv {
	return kv{PDistType: float64(typ), PDistDrive: drive, PDistMix: mix}
}

const (
	fLP24 = iota
	fLP12
	fBP
	fHP
	fNotch
	fAcid
)
const (
	lSine = iota
	lTri
	lSawDn
	lRamp
	lSquare
	lSH
	lDrift
)
const (
	dTube = iota
	dHard
	dFold
	dFuzz
	dSine
	dRect
)
const (
	poly   = 0
	mono   = 1
	legato = 2
)

func mk(name, cat string, knobs [8]PID, parts ...kv) Preset {
	p := Preset{Name: name, Cat: cat, Patch: DefaultPatch(), Knobs: knobs}
	for _, part := range parts {
		for id, v := range part {
			p.Patch[id] = Metas[id].Clamp(v)
		}
	}
	if v, ok := presetVol[name]; ok {
		p.Patch[PVolume] = v
	}
	return p
}

// Presets is the factory bank.
var Presets []Preset

func init() {
	Presets = []Preset{
		// ------------------------------------------------------------ SUB
		mk("Pure Sub", "Sub", KnobsStd, oa(TBasic, 0, 1, 0), flt(fLP12, 500, 0, 0, 0.5), aenv(0.004, 0.3, 1, 0.12)),
		mk("Round Sub", "Sub", KnobsStd, oa(TBasic, 0.2, 1, 0), flt(fLP24, 320, 0.1, 0.15, 0.5), kv{PFDrive: 0.25}, aenv(0.003, 0.4, 0.9, 0.15)),
		mk("808 Long", "Sub", k808, oa(TSaturate, 0.22, 1, 0), flt(fLP12, 900, 0, 0, 0.6), aenv(0.001, 2.6, 0, 0.5),
			menv(0.0005, 0.07, 0, 0.07), mod(1, SrcModEnv, DPitch, 0.5), kv{PGlide: 0.03, PMode: legato, PBass: 3}),
		mk("808 Short", "Sub", k808, oa(TSaturate, 0.3, 1, 0), flt(fLP12, 1400, 0, 0, 0.6), aenv(0.001, 0.55, 0, 0.2),
			menv(0.0005, 0.05, 0, 0.05), mod(1, SrcModEnv, DPitch, 0.55), kv{PBass: 3}),
		mk("808 Distorted", "Sub", k808, oa(TSaturate, 0.62, 1, 0), flt(fLP12, 2600, 0.1, 0, 0.6), aenv(0.001, 2.2, 0, 0.4),
			menv(0.0005, 0.09, 0, 0.09), mod(1, SrcModEnv, DPitch, 0.5), dist(dTube, 0.55, 0.6), kv{PGlide: 0.05, PMode: legato}),
		mk("808 Glide", "Sub", k808, oa(TSaturate, 0.4, 1, 0), flt(fLP12, 1800, 0, 0, 0.6), aenv(0.001, 3.5, 0.3, 0.6),
			menv(0.0005, 0.06, 0, 0.06), mod(1, SrcModEnv, DPitch, 0.45), kv{PGlide: 0.18, PMode: legato}, dist(dTube, 0.35, 0.35)),
		mk("Deep House Sub", "Sub", KnobsStd, oa(TOrgan, 0.3, 1, 0), flt(fLP24, 420, 0.15, 0.25, 0.4), fenv(0.001, 0.18, 0.1, 0.1), aenv(0.002, 0.25, 0.7, 0.08)),
		mk("Click Sub", "Sub", KnobsStd, oa(TBasic, 0.05, 1, 0), flt(fLP12, 5000, 0, 0, 0.3), menv(0.0005, 0.018, 0, 0.02),
			mod(1, SrcModEnv, DNoise, 0.5), mod(2, SrcModEnv, DPitch, 0.45), aenv(0.001, 0.5, 0.85, 0.1)),
		mk("Tape Sub", "Sub", KnobsStd, oa(TBasic, 0.12, 1, 0), ob(TSaturate, 0.2, 0.4, 0), tuneB(0, 6), flt(fLP24, 380, 0.05, 0.1, 0.5),
			lfo2hz(lDrift, 0.7), mod(1, SrcLFO2, DPitch, 0.07), kv{PFDrive: 0.4, PNoise: 0.02}, aenv(0.006, 0.6, 0.9, 0.25)),
		mk("Sub + Fifth", "Sub", KnobsStd, oa(TBasic, 0, 1, 0), ob(TBasic, 0.25, 0.35, 0), tuneB(7, 0), flt(fLP24, 600, 0.1, 0.2, 0.4), aenv(0.003, 0.5, 0.8, 0.15)),

		// ------------------------------------------------------------ CLASSIC
		mk("Mini Bass", "Classic", KnobsStd, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.75, 0.55, -1), flt(fLP24, 170, 0.28, 0.55, 0.35),
			fenv(0.001, 0.32, 0.18, 0.15), aenv(0.001, 0.4, 0.85, 0.1), kv{PFDrive: 0.3}),
		mk("Fat Saw", "Classic", KnobsStd, oa(TBasic, 0.5, 0.75, 0), ob(TBasic, 0.5, 0.75, 0), tuneA(0, -9), tuneB(0, 9), sub(0, -1, 0.4),
			flt(fLP24, 380, 0.2, 0.45, 0.4), fenv(0.001, 0.4, 0.3, 0.2)),
		mk("101 Square", "Classic", KnobsStd, oa(TBasic, 0.75, 0.8, 0), sub(2, -1, 0.45), flt(fLP24, 240, 0.4, 0.5, 0.4),
			fenv(0.001, 0.22, 0.12, 0.15), aenv(0.001, 0.3, 0.8, 0.08)),
		mk("PWM Bass", "Classic", KnobsStd, oa(TPWM, 0.3, 0.85, 0), sub(0, -1, 0.4), lfo2hz(lTri, 0.9), mod(1, SrcLFO2, DPosA, 0.3),
			flt(fLP24, 700, 0.2, 0.35, 0.4), fenv(0.001, 0.5, 0.4, 0.2), kv{PChorus: 0.25}),
		mk("Rubber Bass", "Classic", KnobsStd, oa(TBasic, 0.42, 0.9, 0), flt(fLP24, 120, 0.55, 0.62, 0.4), fenv(0.001, 0.16, 0.05, 0.12),
			aenv(0.001, 0.35, 0.6, 0.1), mod(1, SrcVel, DCutoff, 0.25)),
		mk("Pluck Bass", "Classic", KnobsStd, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.75, 0.4, 0), flt(fLP24, 150, 0.3, 0.7, 0.4),
			fenv(0.0005, 0.12, 0, 0.1), aenv(0.001, 0.28, 0, 0.12), mod(1, SrcVel, DCutoff, 0.3)),
		mk("Juno Bass", "Classic", KnobsStd, oa(TPWM, 0.2, 0.7, 0), ob(TBasic, 0.5, 0.5, 0), sub(2, -1, 0.5), flt(fLP24, 520, 0.15, 0.4, 0.4),
			fenv(0.001, 0.45, 0.3, 0.25), kv{PChorus: 0.55}),
		mk("Solid Bass", "Classic", KnobsStd, oa(TBasic, 0.62, 0.85, 0), ob(TBasic, 0.5, 0.6, 1), tuneB(0, 5), flt(fLP24, 300, 0.2, 0.5, 0.5),
			fenv(0.001, 0.2, 0.25, 0.15), aenv(0.001, 0.2, 0.9, 0.06), kv{PChorus: 0.2}),
		mk("Taurus Pedal", "Classic", KnobsStd, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.8, 0), tuneA(0, -7), tuneB(0, 7), flt(fLP24, 210, 0.3, 0.3, 0.2),
			fenv(0.003, 0.9, 0.5, 0.6), aenv(0.004, 1, 0.9, 0.7), kv{PFDrive: 0.2, PGlide: 0.05}),
		mk("Sync Bass", "Classic", KnobsStd, oa(TSyncSaw, 0.2, 0.9, 0), menv(0.001, 0.3, 0.15, 0.2), mod(1, SrcModEnv, DPosA, 0.6), sub(0, -1, 0.5),
			flt(fLP12, 3200, 0.1, 0.2, 0.4)),
		mk("Reese", "Classic", kReese, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.8, 0), tuneA(0, -14), tuneB(0, 14), flt(fLP24, 650, 0.12, 0, 0.3),
			lfo1hz(lTri, 0.25), mod(1, SrcLFO1, DCutoff, 0.12), aenv(0.004, 0.5, 1, 0.2), kv{PChorus: 0.2}),
		mk("Dark Reese", "Classic", kReese, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.8, 0), uniA(3, 0.4, 0.5), tuneB(0, 21), sub(0, -1, 0.5),
			flt(fLP24, 300, 0.25, 0, 0.3), lfo1hz(lSine, 0.15), mod(1, SrcLFO1, DCutoff, 0.18), aenv(0.006, 0.5, 1, 0.3), kv{PFDrive: 0.35}),
		mk("Hoover", "Classic", kReese, oa(TBasic, 0.5, 0.75, 1), ob(TPWM, 0.35, 0.7, 0), uniA(7, 0.62, 1), uniB(3, 0.5, 0.8), sub(2, -1, 0.3),
			flt(fLP12, 4200, 0.1, 0, 0.3), menv(0.03, 0.2, 0, 0.2), mod(1, SrcModEnv, DPitch, -0.33), lfo1hz(lSine, 5.5), mod(2, SrcLFO1, DPosB, 0.2),
			aenv(0.005, 0.5, 1, 0.25), kv{PChorus: 0.5, PGlide: 0.09}),
		mk("Funk Finger", "Classic", KnobsStd, oa(TBasic, 0.5, 0.85, 0), flt(fLP24, 200, 0.45, 0.5, 0.5), fenv(0.001, 0.14, 0.2, 0.1),
			mod(1, SrcVel, DCutoff, 0.4), aenv(0.001, 0.25, 0.7, 0.07), kv{PVelSens: 0.7}),
		mk("Octave Bass", "Classic", KnobsStd, oa(TOctaves, 0.3, 0.9, 0), sub(0, -1, 0.4), flt(fLP24, 500, 0.2, 0.45, 0.4), fenv(0.001, 0.3, 0.2, 0.2)),
		mk("Detroit Bass", "Classic", KnobsStd, oa(TBasic, 0.7, 0.8, 0), ob(TBasic, 0.5, 0.5, 0), tuneB(7, 0), flt(fLP24, 400, 0.3, 0.4, 0.4),
			fenv(0.001, 0.25, 0.2, 0.2), kv{PDlyMix: 0.18, PDlyFB: 0.4}),

		// ------------------------------------------------------------ ACID
		mk("Acid Saw", "Acid", kAcid, oa(TBasic, 0.5, 0.9, 0), flt(fAcid, 230, 0.82, 0.5, 0.2), fenv(0.0005, 0.22, 0, 0.15), aenv(0.001, 0.3, 0.7, 0.05),
			mod(1, SrcVel, DCutoff, 0.3), kv{PMode: legato, PGlide: 0.06}, dist(dTube, 0.45, 0.35)),
		mk("Acid Square", "Acid", kAcid, oa(TBasic, 0.75, 0.9, 0), flt(fAcid, 200, 0.85, 0.55, 0.2), fenv(0.0005, 0.18, 0, 0.15), aenv(0.001, 0.3, 0.7, 0.05),
			mod(1, SrcVel, DCutoff, 0.3), kv{PMode: legato, PGlide: 0.06}, dist(dTube, 0.4, 0.3)),
		mk("Acid Screamer", "Acid", kAcid, oa(TBasic, 0.5, 0.9, 0), flt(fAcid, 320, 0.93, 0.65, 0.3), fenv(0.0005, 0.3, 0.1, 0.2), aenv(0.001, 0.3, 0.8, 0.05),
			mod(1, SrcVel, DCutoff, 0.35), kv{PMode: legato, PGlide: 0.08}, dist(dHard, 0.6, 0.7)),
		mk("Acid Wobble", "Acid", kAcid, oa(TBasic, 0.55, 0.9, 0), flt(fAcid, 300, 0.8, 0.3, 0.2), fenv(0.0005, 0.2, 0, 0.15), lfo1(lTri, "1/4"),
			mod(1, SrcLFO1, DCutoff, 0.3), kv{PMode: legato, PGlide: 0.05}, dist(dTube, 0.4, 0.3)),
		mk("Liquid Acid", "Acid", kAcid, oa(TReso, 0.3, 0.9, 0), flt(fAcid, 260, 0.7, 0.45, 0.3), fenv(0.0005, 0.25, 0.05, 0.2), menv(0.001, 0.4, 0, 0.3),
			mod(1, SrcModEnv, DPosA, 0.5), kv{PMode: legato, PGlide: 0.07, PDlyMix: 0.2, PDlyFB: 0.45}),
		mk("Rave Acid", "Acid", kAcid, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.75, 0.6, 1), flt(fAcid, 400, 0.88, 0.5, 0.3), fenv(0.0005, 0.15, 0, 0.1),
			mod(1, SrcVel, DCutoff, 0.3), kv{PMode: legato, PGlide: 0.04, PDlyMix: 0.22}, dist(dFuzz, 0.5, 0.5)),

		// ------------------------------------------------------------ FM / DIGITAL
		mk("DX Bass", "FM / Digital", kFM, oa(TFM1, 0.08, 0.95, 0), menv(0.0005, 0.22, 0.05, 0.15), mod(1, SrcModEnv, DPosA, 0.6), mod(2, SrcVel, DPosA, 0.2),
			flt(fLP12, 9000, 0, 0, 0.3), aenv(0.001, 0.6, 0.6, 0.1)),
		mk("Lately Bass", "FM / Digital", kFM, oa(TFM2, 0.1, 0.9, 0), ob(TFM1, 0.3, 0.4, 0), menv(0.0005, 0.15, 0.1, 0.12), mod(1, SrcModEnv, DPosA, 0.55),
			sub(0, -1, 0.3), flt(fLP12, 7000, 0, 0, 0.3), aenv(0.001, 0.4, 0.7, 0.08)),
		mk("Donk", "FM / Digital", kFM, oa(TFM3, 0.05, 0.95, 0), menv(0.0005, 0.07, 0, 0.07), mod(1, SrcModEnv, DPosA, 0.85), flt(fLP12, 8000, 0, 0, 0.3),
			aenv(0.001, 0.22, 0, 0.1), kv{PBass: 2}),
		mk("FM Thump", "FM / Digital", kFM, oa(TBasic, 0, 0.95, 0), ob(TBasic, 0, 0, 1), kv{PFM: 0.15}, menv(0.0005, 0.12, 0, 0.1), mod(1, SrcModEnv, DFM, 0.75),
			flt(fLP12, 6000, 0, 0, 0.3), aenv(0.001, 0.5, 0.5, 0.12)),
		mk("FM Growl Bass", "FM / Digital", kFM, oa(TBasic, 0.1, 0.9, 0), ob(TBasic, 0.3, 0, 0), tuneB(7, 0), kv{PFM: 0.4}, lfo1(lSine, "1/8"),
			mod(1, SrcLFO1, DFM, 0.35), flt(fLP24, 2400, 0.2, 0, 0.3), sub(0, -1, 0.4)),
		mk("Metal Bass", "FM / Digital", kFM, oa(TMetal, 0.2, 0.9, 0), menv(0.0005, 0.3, 0.1, 0.2), mod(1, SrcModEnv, DPosA, 0.5), sub(0, -1, 0.5),
			flt(fLP12, 5000, 0.1, 0.2, 0.3)),
		mk("Digital Pluck", "FM / Digital", kFM, oa(TDigital, 0.1, 0.9, 0), menv(0.0005, 0.25, 0, 0.2), mod(1, SrcModEnv, DPosA, 0.8), flt(fLP24, 900, 0.2, 0.5, 0.4),
			fenv(0.0005, 0.2, 0.1, 0.15), aenv(0.001, 0.4, 0.3, 0.15), sub(0, -1, 0.35)),
		mk("Organ Bass", "FM / Digital", kFM, oa(TOrgan, 0.45, 0.95, 0), flt(fLP12, 4000, 0, 0, 0.3), menv(0.0005, 0.02, 0, 0.02), mod(1, SrcModEnv, DNoise, 0.25),
			aenv(0.001, 0.2, 0.9, 0.05)),
		mk("Garage Organ", "FM / Digital", kFM, oa(TOrgan, 0.75, 0.9, 0), ob(TFM2, 0.15, 0.3, 1), flt(fLP12, 3000, 0.1, 0.2, 0.3), fenv(0.0005, 0.1, 0, 0.1),
			aenv(0.001, 0.25, 0.5, 0.06), kv{PMode: poly}),
		mk("CZ Reso Bass", "FM / Digital", kFM, oa(TPhaseDist, 0.2, 0.9, 0), menv(0.0005, 0.3, 0.1, 0.2), mod(1, SrcModEnv, DPosA, 0.7), sub(0, -1, 0.4),
			flt(fLP12, 8000, 0, 0, 0.3)),
		mk("Bell Bass", "FM / Digital", kFM, oa(TFM3, 0.3, 0.8, 0), ob(TBasic, 0, 0.6, 0), menv(0.0005, 0.6, 0, 0.5), mod(1, SrcModEnv, DPosA, 0.4),
			flt(fLP12, 9000, 0, 0, 0.3), aenv(0.001, 1.2, 0.2, 0.4), kv{PRevMix: 0.15}),
		mk("Harmonic Scan", "FM / Digital", kFM, oa(THarmonics, 0, 0.9, 0), lfo1(lRamp, "1/2"), mod(1, SrcLFO1, DPosA, 0.5), kv{PAPos: 0.5}, sub(0, 0, 0.5),
			flt(fLP12, 9000, 0, 0, 0.3)),

		// ------------------------------------------------------------ WOBBLE
		mk("Wobble 1/8", "Wobble", kWob, oa(TBasic, 0.55, 0.8, 0), ob(TBasic, 0.75, 0.5, 0), tuneB(0, 8), sub(0, -1, 0.5), flt(fLP24, 420, 0.35, 0, 0.2),
			lfo1(lSine, "1/8"), mod(1, SrcLFO1, DCutoff, 0.5), aenv(0.003, 0.3, 1, 0.1), dist(dTube, 0.4, 0.3)),
		mk("Wobble Triplet", "Wobble", kWob, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.6, 0), tuneB(0, 12), sub(0, -1, 0.5), flt(fLP24, 380, 0.45, 0, 0.2),
			lfo1(lTri, "1/8T"), mod(1, SrcLFO1, DCutoff, 0.55), aenv(0.003, 0.3, 1, 0.1), dist(dTube, 0.45, 0.35)),
		mk("Slow Lurch", "Wobble", kWob, oa(TBasic, 0.5, 0.8, 0), uniA(3, 0.3, 0.6), sub(2, -1, 0.4), flt(fLP24, 240, 0.4, 0, 0.2),
			lfo1(lRamp, "1/4"), mod(1, SrcLFO1, DCutoff, 0.6), aenv(0.003, 0.3, 1, 0.1), dist(dTube, 0.5, 0.4)),
		mk("Vowel Wobble", "Wobble", kWob, oa(TVowel, 0.5, 0.9, 0), sub(0, -1, 0.5), flt(fLP12, 5000, 0.1, 0, 0.2), lfo1(lSine, "1/8"),
			mod(1, SrcLFO1, DPosA, 0.5), aenv(0.003, 0.3, 1, 0.1), dist(dTube, 0.4, 0.3)),
		mk("Yoy Bass", "Wobble", kWob, oa(TYoy, 0.5, 0.9, 0), ob(TBasic, 0.5, 0.3, 0), sub(0, -1, 0.5), flt(fLP12, 6000, 0.1, 0, 0.2), lfo1(lTri, "1/4"),
			mod(1, SrcLFO1, DPosA, 0.5), aenv(0.003, 0.3, 1, 0.1), dist(dTube, 0.5, 0.4)),
		mk("Gate Wobble", "Wobble", kWob, oa(TFoldSaw, 0.3, 0.85, 0), sub(0, -1, 0.5), flt(fLP24, 1500, 0.3, 0, 0.2), lfo1(lSquare, "1/8"),
			mod(1, SrcLFO1, DAmp, 0.5), lfo2(lSine, "1 bar"), mod(2, SrcLFO2, DCutoff, 0.35), kv{PAmpS: 1}),
		mk("Wheel Wobble", "Wobble", [8]PID{PCutoff, PReso, PL1Rate, PM1Amt, PAPos, PM2Amt, PDistMix, PSubLevel}, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.75, 0.6, 0), tuneB(0, 10), sub(0, -1, 0.5),
			flt(fLP24, 350, 0.4, 0, 0.2), lfo1hz(lSine, 2), kv{PL1Retrig: 1}, mod(1, SrcLFO1, DCutoff, 0.5), mod(2, SrcWheel, DL1Rate, 0.8), kv{PAmpS: 1},
			dist(dTube, 0.4, 0.3)),
		mk("Band Wobble", "Wobble", kWob, oa(TBasic, 0.5, 0.9, 0), uniA(3, 0.25, 0.7), sub(0, -1, 0.6), flt(fBP, 500, 0.6, 0, 0.2), lfo1(lSine, "1/8"),
			mod(1, SrcLFO1, DCutoff, 0.45), kv{PAmpS: 1}, dist(dTube, 0.5, 0.4)),
		mk("S&H Step", "Wobble", kWob, oa(TBasic, 0.6, 0.85, 0), sub(2, -1, 0.4), flt(fLP24, 600, 0.6, 0, 0.2), lfo1(lSH, "1/16"),
			mod(1, SrcLFO1, DCutoff, 0.5), kv{PAmpS: 1}, dist(dTube, 0.35, 0.25)),
		mk("Fold Wobble", "Wobble", kWob, oa(TFold, 0.3, 0.9, 0), sub(0, -1, 0.5), flt(fLP12, 5000, 0.1, 0, 0.2), lfo1(lSine, "1/8"),
			mod(1, SrcLFO1, DPosA, 0.4), kv{PAmpS: 1}),
		mk("Sync Wobble", "Wobble", kWob, oa(TSyncSaw, 0.4, 0.9, 0), sub(0, -1, 0.5), flt(fLP24, 3000, 0.2, 0, 0.2), lfo1(lTri, "1/4"),
			mod(1, SrcLFO1, DPosA, 0.45), kv{PAmpS: 1}, dist(dTube, 0.4, 0.3)),
		mk("Dub Siren Bass", "Wobble", kWob, oa(TBasic, 0.7, 0.85, 0), sub(0, -1, 0.5), flt(fLP24, 500, 0.7, 0, 0.3), lfo1(lSine, "1/2"),
			mod(1, SrcLFO1, DCutoff, 0.55), kv{PAmpS: 1, PDlyMix: 0.3, PDlyFB: 0.55, PRevMix: 0.15}),

		// ------------------------------------------------------------ GROWL
		mk("Growl", "Growl", kGrowl, oa(TGrowl, 0.5, 0.9, 0), ob(TBasic, 0.5, 0.25, 0), tuneB(0, 10), sub(0, -1, 0.55), flt(fLP12, 7000, 0.15, 0, 0.2),
			lfo1(lSine, "1/4"), mod(1, SrcLFO1, DPosA, 0.5), kv{PAmpS: 1}, dist(dTube, 0.6, 0.6)),
		mk("Talking Growl", "Growl", kGrowl, oa(TVowel, 0.5, 0.85, 0), ob(TGrowl, 0.5, 0.5, 0), sub(0, -1, 0.5), flt(fBP, 1200, 0.35, 0, 0.2),
			lfo1(lTri, "1/4"), mod(1, SrcLFO1, DPosA, 0.5), mod(2, SrcLFO1, DCutoff, 0.25), mod(3, SrcLFO1, DPosB, -0.5), kv{PAmpS: 1},
			dist(dHard, 0.5, 0.5)),
		mk("FM Growl", "Growl", kGrowl, oa(TFM1, 0.3, 0.9, 0), ob(TBasic, 0.2, 0, 1), kv{PFM: 0.35}, sub(0, -1, 0.5), flt(fLP24, 2600, 0.3, 0, 0.2),
			lfo1(lSine, "1/8"), mod(1, SrcLFO1, DPosA, 0.4), mod(2, SrcLFO1, DFM, 0.25), kv{PAmpS: 1}, dist(dTube, 0.6, 0.55)),
		mk("Tear Out", "Growl", kGrowl, oa(TFoldSaw, 0.5, 0.9, 0), ob(TBasic, 0.75, 0, 0), kv{PFM: 0.3}, sub(2, -1, 0.4), flt(fNotch, 900, 0.5, 0, 0.2),
			lfo1(lSawDn, "1/8"), mod(1, SrcLFO1, DPosA, 0.5), mod(2, SrcLFO1, DCutoff, 0.4), kv{PAmpS: 1}, dist(dHard, 0.7, 0.7)),
		mk("Riddim Chop", "Growl", kGrowl, oa(TBasic, 0.75, 0.9, 0), ob(TBasic, 0.72, 0, 0), tuneB(12, 0), kv{PFM: 0.45}, sub(2, -1, 0.35),
			flt(fLP24, 1800, 0.45, 0, 0.2), lfo1(lSquare, "1/8T"), mod(1, SrcLFO1, DFM, 0.3), mod(2, SrcLFO1, DCutoff, 0.3), kv{PAmpS: 1},
			dist(dHard, 0.55, 0.6)),
		mk("Screech", "Growl", kGrowl, oa(TScreech, 0.4, 0.85, 1), ob(TSyncSaw, 0.5, 0.4, 0), sub(0, -1, 0.5), flt(fBP, 2400, 0.5, 0, 0.2),
			lfo1(lRamp, "1/4"), mod(1, SrcLFO1, DPosA, 0.6), mod(2, SrcLFO1, DCutoff, 0.3), kv{PAmpS: 1}, dist(dHard, 0.65, 0.65)),
		mk("Robo Mouth", "Growl", kGrowl, oa(THarmonics, 0.4, 0.9, 0), ob(TVowel, 0.3, 0.5, 0), sub(0, -1, 0.5), flt(fLP12, 6000, 0.2, 0, 0.2),
			lfo1(lSH, "1/16"), mod(1, SrcLFO1, DPosA, 0.5), mod(2, SrcLFO1, DPosB, 0.4), kv{PAmpS: 1}, dist(dFold, 0.3, 0.4)),
		mk("Monster", "Growl", kGrowl, oa(TGrowl, 0.3, 0.9, 0), ob(TFM2, 0.4, 0.5, -1), uniA(3, 0.3, 0.8), kv{PFM: 0.25}, sub(0, -1, 0.5),
			flt(fLP24, 1500, 0.4, 0, 0.2), lfo1(lTri, "1/2"), mod(1, SrcLFO1, DPosA, 0.6), mod(2, SrcLFO1, DCutoff, 0.4), kv{PAmpS: 1},
			dist(dFuzz, 0.6, 0.6)),
		mk("Machine Gun", "Growl", kGrowl, oa(TScreech, 0.2, 0.9, 0), ob(TBasic, 0.75, 0.4, 0), sub(2, -1, 0.4), flt(fLP24, 2200, 0.3, 0, 0.2),
			lfo1(lSawDn, "1/16"), mod(1, SrcLFO1, DAmp, 0.6), mod(2, SrcLFO1, DPosA, 0.4), kv{PAmpS: 1}, dist(dHard, 0.6, 0.6)),
		mk("Vowel Scream", "Growl", kGrowl, oa(TYoy, 0.7, 0.9, 0), ob(TVowel, 0.2, 0.6, 1), uniA(3, 0.25, 0.7), sub(0, -1, 0.5), flt(fLP12, 8000, 0.2, 0, 0.2),
			lfo1(lSine, "1/8T"), mod(1, SrcLFO1, DPosA, -0.5), mod(2, SrcLFO1, DPosB, 0.5), kv{PAmpS: 1}, dist(dSine, 0.5, 0.5)),

		// ------------------------------------------------------------ NEURO / DNB
		mk("Neuro Bass", "Neuro / DnB", kGrowl, oa(TNeuro, 0.4, 0.9, 0), uniA(3, 0.3, 0.7), sub(0, -1, 0.55), flt(fNotch, 700, 0.6, 0, 0.2),
			lfo1(lTri, "1/16T"), mod(1, SrcLFO1, DCutoff, 0.35), lfo2hz(lDrift, 0.6), mod(2, SrcLFO2, DPosA, 0.4), kv{PAmpS: 1}, dist(dFold, 0.35, 0.5)),
		mk("Neuro Reese", "Neuro / DnB", kReese, oa(TBasic, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.8, 0), uniA(5, 0.45, 0.9), tuneB(0, 18), sub(0, -1, 0.55),
			flt(fNotch, 500, 0.7, 0, 0.2), lfo1hz(lTri, 0.35), mod(1, SrcLFO1, DCutoff, 0.4), kv{PAmpS: 1}, dist(dTube, 0.55, 0.5)),
		mk("Phase Reese", "Neuro / DnB", kReese, oa(TComb, 0.3, 0.9, 0), ob(TBasic, 0.5, 0.6, 0), tuneB(0, 16), sub(0, -1, 0.5), flt(fLP24, 1400, 0.2, 0, 0.2),
			lfo1hz(lSine, 0.3), mod(1, SrcLFO1, DPosA, 0.4), kv{PAmpS: 1}, dist(dTube, 0.45, 0.4)),
		mk("Jump Up", "Neuro / DnB", kGrowl, oa(TFM2, 0.4, 0.9, 0), ob(TBasic, 0.75, 0.4, 0), sub(2, -1, 0.4), flt(fLP24, 1200, 0.4, 0, 0.2),
			lfo1(lSquare, "1/8"), mod(1, SrcLFO1, DPosA, 0.4), mod(2, SrcLFO1, DCutoff, 0.3), kv{PAmpS: 1}, dist(dHard, 0.5, 0.5)),
		mk("Foghorn", "Neuro / DnB", KnobsStd, oa(TBasic, 0.5, 0.85, 0), ob(TBasic, 0.5, 0.7, 0), tuneB(0, 11), sub(0, -1, 0.5), flt(fLP24, 110, 0.3, 0.55, 0.2),
			fenv(0.25, 0.8, 0.6, 0.3), aenv(0.02, 0.5, 1, 0.3), kv{PFDrive: 0.6}, dist(dTube, 0.5, 0.4)),
		mk("Dread Bass", "Neuro / DnB", KnobsStd, oa(TFM1, 0.3, 0.9, 0), sub(0, -1, 0.6), flt(fLP24, 500, 0.2, 0.3, 0.3), fenv(0.001, 0.3, 0.3, 0.2),
			menv(0.001, 0.12, 0, 0.1), mod(1, SrcModEnv, DPitch, 0.3), aenv(0.002, 0.5, 0.9, 0.2)),
		mk("Jungle Sub", "Neuro / DnB", k808, oa(TSaturate, 0.15, 1, 0), flt(fLP12, 700, 0, 0, 0.5), aenv(0.002, 1.4, 0.5, 0.35), menv(0.0005, 0.1, 0, 0.1),
			mod(1, SrcModEnv, DPitch, 0.4), kv{PGlide: 0.04}),
		mk("Tech Stab", "Neuro / DnB", kGrowl, oa(TNeuro, 0.7, 0.9, 0), ob(TFoldSaw, 0.4, 0.5, 0), sub(0, -1, 0.5), flt(fLP24, 300, 0.4, 0.6, 0.2),
			fenv(0.0005, 0.14, 0, 0.1), aenv(0.001, 0.22, 0, 0.1), menv(0.0005, 0.15, 0, 0.1), mod(1, SrcModEnv, DPosA, -0.5), dist(dHard, 0.5, 0.5)),
		mk("Hornet", "Neuro / DnB", kReese, oa(TPWM, 0.5, 0.8, 0), ob(TBasic, 0.5, 0.7, 1), uniA(7, 0.5, 1), uniB(3, 0.4, 0.8), sub(2, -1, 0.4),
			flt(fLP24, 2000, 0.3, 0, 0.2), lfo1hz(lSine, 6), mod(1, SrcLFO1, DPitch, 0.1), kv{PAmpS: 1}, dist(dFuzz, 0.5, 0.5)),
		mk("Razor", "Neuro / DnB", kGrowl, oa(TSawFilter, 0.5, 0.9, 0), uniA(3, 0.2, 0.6), sub(0, -1, 0.5), flt(fHP, 90, 0.3, 0, 0), lfo1(lRamp, "1/8"),
			mod(1, SrcLFO1, DPosA, 0.5), kv{PAmpS: 1}, dist(dHard, 0.55, 0.55)),

		// ------------------------------------------------------------ WEIRD
		mk("Bit Bass", "Weird", kSpace, oa(TCrush, 0.4, 0.9, 0), sub(2, -1, 0.4), flt(fLP12, 5000, 0.1, 0, 0.3), menv(0.001, 0.3, 0.2, 0.2), mod(1, SrcModEnv, DPosA, 0.4),
			kv{PCrush: 0.35}),
		mk("Laser Zap", "Weird", kSpace, oa(TSyncSine, 0.4, 0.9, 0), menv(0.0005, 0.18, 0, 0.15), mod(1, SrcModEnv, DPitch, 0.9), mod(2, SrcModEnv, DPosA, 0.5),
			flt(fLP12, 9000, 0.2, 0, 0.3), aenv(0.001, 0.4, 0, 0.2), kv{PDlyMix: 0.2}),
		mk("Alien Talk", "Weird", kSpace, oa(TVowel, 0.5, 0.9, 0), ob(TYoy, 0.5, 0.6, 1), lfo1(lSH, "1/16"), mod(1, SrcLFO1, DPosA, 0.5), lfo2(lDrift, "1/4"),
			mod(2, SrcLFO2, DPosB, 0.5), sub(0, -1, 0.4), flt(fBP, 1500, 0.4, 0, 0.3), kv{PAmpS: 1, PDlyMix: 0.2}),
		mk("Broken Speaker", "Weird", kSpace, oa(TRectify, 0.6, 0.9, 0), sub(0, -1, 0.6), flt(fLP24, 900, 0.3, 0.3, 0.3), kv{PCrush: 0.5, PFDrive: 0.8},
			dist(dRect, 0.5, 0.5)),
		mk("Drone Engine", "Weird", kSpace, oa(TOctaves, 0.6, 0.8, 0), ob(TMetal, 0.3, 0.4, 0), uniA(5, 0.3, 1), lfo1hz(lDrift, 0.2), mod(1, SrcLFO1, DPosA, 0.4),
			lfo2hz(lDrift, 0.13), mod(2, SrcLFO2, DCutoff, 0.3), flt(fLP24, 600, 0.4, 0, 0.2), aenv(0.6, 1, 1, 1.5), kv{PRevMix: 0.4, PRevSize: 0.9}),
		mk("Glitch Step", "Weird", kSpace, oa(TDigital, 0.5, 0.9, 0), lfo1(lSH, "1/16"), mod(1, SrcLFO1, DPosA, 0.5), mod(2, SrcLFO1, DPitchA, 0.45),
			sub(0, -1, 0.5), flt(fLP12, 6000, 0.3, 0, 0.3), kv{PAmpS: 1, PCrush: 0.2}),
		mk("Space Sub", "Weird", kSpace, oa(TBasic, 0.1, 1, 0), flt(fLP24, 900, 0.3, 0.3, 0.3), fenv(0.001, 0.6, 0, 0.5), aenv(0.002, 0.8, 0.4, 0.6),
			kv{PDlyMix: 0.35, PDlyFB: 0.6, PRevMix: 0.3, PRevSize: 0.8}),
		mk("Comb Monster", "Weird", kSpace, oa(TComb, 0.5, 0.9, 0), ob(TComb, 0.5, 0.7, -1), lfo1(lSine, "1 bar"), mod(1, SrcLFO1, DPosA, 0.5), mod(2, SrcLFO1, DPosB, -0.5),
			flt(fLP12, 7000, 0.2, 0, 0.3), kv{PAmpS: 1}, dist(dFold, 0.4, 0.5)),
		mk("Fold City", "Weird", kSpace, oa(TFold, 0.2, 0.9, 0), menv(0.3, 1.2, 0.3, 0.4), mod(1, SrcModEnv, DPosA, 0.7), sub(0, -1, 0.5), flt(fLP12, 8000, 0, 0, 0.3),
			dist(dFold, 0.5, 0.4)),
		mk("Rubber Chicken", "Weird", kSpace, oa(TYoy, 0.2, 0.9, 1), menv(0.0005, 0.25, 0, 0.2), mod(1, SrcModEnv, DPosA, 0.8), mod(2, SrcModEnv, DPitch, 0.5),
			flt(fBP, 1600, 0.5, 0, 0.5), aenv(0.001, 0.35, 0, 0.15)),
		mk("Motor Idle", "Weird", kSpace, oa(TRectify, 0.3, 0.9, -1), ob(TBasic, 0.75, 0.5, 0), lfo1hz(lSawDn, 14), mod(1, SrcLFO1, DAmp, 0.5), mod(2, SrcWheel, DL1Rate, 0.6),
			flt(fLP24, 700, 0.3, 0, 0.3), kv{PAmpS: 1, PFDrive: 0.5}),

		// ------------------------------------------------------------ POLY / STAB
		mk("Rave Stab", "Poly / Stab", kReese, oa(TBasic, 0.5, 0.75, 1), ob(TBasic, 0.5, 0.7, 1), uniA(5, 0.5, 1), tuneB(7, 0), flt(fLP12, 3500, 0.2, 0.3, 0.3),
			fenv(0.001, 0.3, 0.2, 0.2), aenv(0.002, 0.35, 0.5, 0.2), kv{PMode: poly, PChorus: 0.4, PRevMix: 0.15}),
		mk("Dub Chord", "Poly / Stab", kSpace, oa(TBasic, 0.5, 0.7, 1), ob(TBasic, 0.75, 0.5, 1), tuneB(0, 7), flt(fLP12, 600, 0.4, 0.45, 0.3), fenv(0.001, 0.3, 0.1, 0.25),
			aenv(0.002, 0.45, 0, 0.3), kv{PMode: poly, PDlyMix: 0.45, PDlyFB: 0.6, PRevMix: 0.25, PDlyDiv: 3}),
		mk("Deep Chord", "Poly / Stab", kSpace, oa(TOrgan, 0.5, 0.8, 1), ob(TBasic, 0.25, 0.5, 1), flt(fLP24, 900, 0.2, 0.35, 0.3), fenv(0.001, 0.35, 0.1, 0.3),
			aenv(0.003, 0.5, 0.4, 0.3), kv{PMode: poly, PChorus: 0.35, PRevMix: 0.2}),
		mk("Poly Reese", "Poly / Stab", kReese, oa(TBasic, 0.5, 0.75, 0), ob(TBasic, 0.5, 0.75, 0), uniA(3, 0.35, 0.9), tuneB(0, 13), flt(fLP24, 900, 0.15, 0.1, 0.3),
			aenv(0.01, 0.6, 1, 0.4), kv{PMode: poly, PChorus: 0.3}),
		mk("Sub Pad", "Poly / Stab", kSpace, oa(TBasic, 0.2, 0.8, 0), ob(TSawFilter, 0.3, 0.5, 1), uniB(5, 0.35, 1), lfo2hz(lSine, 0.2), mod(1, SrcLFO2, DPosB, 0.25),
			flt(fLP24, 1200, 0.1, 0.1, 0.3), aenv(0.35, 1, 0.9, 1.2), kv{PMode: poly, PRevMix: 0.35, PRevSize: 0.8}),
		mk("Detroit Stab", "Poly / Stab", kSpace, oa(TBasic, 0.5, 0.75, 1), ob(TBasic, 0.75, 0.6, 0), tuneB(0, -6), flt(fLP24, 500, 0.4, 0.5, 0.3), fenv(0.001, 0.18, 0, 0.15),
			aenv(0.001, 0.25, 0, 0.2), kv{PMode: poly, PDlyMix: 0.25, PRevMix: 0.2}),
		mk("Hoover Chord", "Poly / Stab", kReese, oa(TBasic, 0.5, 0.75, 1), ob(TPWM, 0.35, 0.6, 0), uniA(7, 0.6, 1), uniB(3, 0.5, 0.8), flt(fLP12, 5000, 0.1, 0, 0.3),
			menv(0.03, 0.2, 0, 0.2), mod(1, SrcModEnv, DPitch, -0.3), aenv(0.005, 0.5, 0.9, 0.3), kv{PMode: poly, PChorus: 0.5}),
	}
}
