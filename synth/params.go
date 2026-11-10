// Package synth is the WaveBass sound engine: wavetable oscillators, filter, modulation,
// effects, arpeggiator / note repeat, loop recorder and the preset bank. It has no platform
// dependencies, so it can be rendered and tested offline.
package synth

import (
	"fmt"
	"math"
)

const SampleRate = 48000

// PID identifies one synth parameter.
type PID int

const (
	PATable PID = iota
	PAPos
	PAOct
	PASemi
	PAFine
	PALevel
	PAUni
	PADetune
	PASpread

	PBTable
	PBPos
	PBOct
	PBSemi
	PBFine
	PBLevel
	PBUni
	PBDetune
	PBSpread

	PFM

	PSubShape
	PSubOct
	PSubLevel
	PNoise

	PFType
	PCutoff
	PReso
	PFDrive
	PFEnv
	PFKey

	PAmpA
	PAmpD
	PAmpS
	PAmpR
	PFltA
	PFltD
	PFltS
	PFltR
	PModA
	PModD
	PModS
	PModR

	PL1Shape
	PL1Sync
	PL1Rate
	PL1Div
	PL1Retrig
	PL2Shape
	PL2Sync
	PL2Rate
	PL2Div
	PL2Retrig

	PM1Src
	PM1Dst
	PM1Amt
	PM2Src
	PM2Dst
	PM2Amt
	PM3Src
	PM3Dst
	PM3Amt
	PM4Src
	PM4Dst
	PM4Amt
	PM5Src
	PM5Dst
	PM5Amt
	PM6Src
	PM6Dst
	PM6Amt

	PMode
	PGlide
	PBend
	PVelSens

	PDistType
	PDistDrive
	PDistMix
	PCrush
	PChorus
	PDlyDiv
	PDlyFB
	PDlyMix
	PRevSize
	PRevMix
	PBass
	PWidth
	PVolume

	PArpMode
	PArpRate
	PArpOct
	PArpGate
	PArpSwing
	PArpLatch

	NumParams
)

const ModSlots = 6

// Kind is how a parameter maps a 0..1 knob position onto its value.
type Kind int

const (
	KLin Kind = iota
	KExp
	KSq
	KInt
	KChoice
)

// Meta describes one parameter.
type Meta struct {
	Key      string // stable id used in saved files
	Name     string // short label
	Long     string // full name (for knob assignment)
	Sec      string
	Min, Max float64
	Def      float64
	Kind     Kind
	Choices  []string
	Unit     string
	Global   bool // performance setting: not changed when a preset loads
	NoKnob   bool // not offered as a hardware-knob target
}

// Metas is indexed by PID.
var Metas [NumParams]Meta

// Modulation sources.
const (
	SrcNone = iota
	SrcLFO1
	SrcLFO2
	SrcModEnv
	SrcFltEnv
	SrcAmpEnv
	SrcVel
	SrcKey
	SrcWheel
	SrcAT
	SrcRand
	SrcL1Wheel
	NumSrc
)

var SrcNames = []string{"-", "LFO 1", "LFO 2", "Mod Env", "Filt Env", "Amp Env", "Velocity", "Key Track", "Mod Wheel", "Aftertouch", "Random", "LFO1 x Wheel"}

// Modulation destinations.
const (
	DNone = iota
	DPosA
	DPosB
	DCutoff
	DReso
	DPitch
	DPitchA
	DPitchB
	DLevelA
	DLevelB
	DSub
	DNoise
	DFM
	DFDrive
	DAmp
	DPan
	DDetune
	DL1Rate
	DL2Rate
	DDist
	NumDst
)

var DstNames = []string{"-", "WT Pos A", "WT Pos B", "Cutoff", "Resonance", "Pitch", "Pitch A", "Pitch B", "Level A", "Level B", "Sub Level", "Noise", "FM B>A", "Filt Drive", "Volume", "Pan", "Detune", "LFO1 Rate", "LFO2 Rate", "Distortion"}

// Div is a tempo-synced note length.
type Div struct {
	Name  string
	Beats float64 // quarter notes
}

// LFODivs are LFO cycle lengths, slowest first.
var LFODivs = []Div{
	{"8 bars", 32}, {"4 bars", 16}, {"2 bars", 8}, {"1 bar", 4}, {"1/2.", 3}, {"1/2", 2}, {"1/4.", 1.5},
	{"1/2T", 4.0 / 3}, {"1/4", 1}, {"1/8.", 0.75}, {"1/4T", 2.0 / 3}, {"1/8", 0.5}, {"1/8T", 1.0 / 3},
	{"1/16", 0.25}, {"1/16T", 1.0 / 6}, {"1/32", 0.125},
}

// DelayDivs are delay times.
var DelayDivs = []Div{
	{"1/2", 2}, {"1/4.", 1.5}, {"1/4", 1}, {"1/8.", 0.75}, {"1/4T", 2.0 / 3}, {"1/8", 0.5}, {"1/8T", 1.0 / 3},
	{"1/16", 0.25}, {"1/16T", 1.0 / 6}, {"1/32", 0.125},
}

// ArpRates are arpeggiator / note-repeat step lengths.
var ArpRates = []Div{
	{"1/4", 1}, {"1/8", 0.5}, {"1/8T", 1.0 / 3}, {"1/16", 0.25}, {"1/16T", 1.0 / 6}, {"1/32", 0.125}, {"1/32T", 1.0 / 12},
}

const (
	ArpOff = iota
	ArpRepeat
	ArpUp
	ArpDown
	ArpUpDown
	ArpRandom
	ArpPlayed
)

var ArpModes = []string{"Off", "Repeat", "Up", "Down", "Up-Down", "Random", "Played"}

var (
	FilterTypes = []string{"LP 24", "LP 12", "Band", "High", "Notch", "Acid"}
	LFOShapes   = []string{"Sine", "Tri", "Saw", "Ramp", "Square", "S&H", "Drift"}
	SubShapes   = []string{"Sine", "Tri", "Square", "Saw"}
	VoiceModes  = []string{"Poly", "Mono", "Legato"}
	DistTypes   = []string{"Tube", "Hard", "Fold", "Fuzz", "Sine", "Rect"}
	offOn       = []string{"Off", "On"}
)

func def(id PID, key, name, long, sec string, min, max, d float64, k Kind, unit string) {
	Metas[id] = Meta{Key: key, Name: name, Long: long, Sec: sec, Min: min, Max: max, Def: d, Kind: k, Unit: unit}
}

func choice(id PID, key, name, long, sec string, ch []string, d int) {
	Metas[id] = Meta{Key: key, Name: name, Long: long, Sec: sec, Min: 0, Max: float64(len(ch) - 1), Def: float64(d), Kind: KChoice, Choices: ch}
}

func init() {
	names := make([]string, len(tableDefs))
	for i, t := range tableDefs {
		names[i] = t.name
	}
	osc := func(base PID, k, n string, lvl float64) {
		choice(base, k+".tab", "TABLE", "Osc "+n+" Wavetable", "OSC "+n, names, TBasic)
		def(base+1, k+".pos", "POS", "Osc "+n+" WT Position", "OSC "+n, 0, 1, 0.5, KLin, "%")
		def(base+2, k+".oct", "OCT", "Osc "+n+" Octave", "OSC "+n, -3, 3, 0, KInt, "")
		def(base+3, k+".semi", "SEMI", "Osc "+n+" Semitone", "OSC "+n, -12, 12, 0, KInt, "st")
		def(base+4, k+".fine", "FINE", "Osc "+n+" Fine Tune", "OSC "+n, -100, 100, 0, KLin, "ct")
		def(base+5, k+".lvl", "LEVEL", "Osc "+n+" Level", "OSC "+n, 0, 1, lvl, KLin, "%")
		def(base+6, k+".uni", "UNISON", "Osc "+n+" Unison Voices", "OSC "+n, 1, 7, 1, KInt, "")
		def(base+7, k+".det", "DETUNE", "Osc "+n+" Unison Detune", "OSC "+n, 0, 1, 0.3, KLin, "%")
		def(base+8, k+".spr", "SPREAD", "Osc "+n+" Stereo Spread", "OSC "+n, 0, 1, 0.6, KLin, "%")
	}
	osc(PATable, "a", "A", 0.8)
	osc(PBTable, "b", "B", 0)
	def(PFM, "fm", "FM B>A", "FM Amount (B>A)", "SUB / NOISE / FM", 0, 1, 0, KLin, "%")

	choice(PSubShape, "sub.shape", "SUB", "Sub Shape", "SUB / NOISE / FM", SubShapes, 0)
	def(PSubOct, "sub.oct", "SUB OCT", "Sub Octave", "SUB / NOISE / FM", -2, 0, -1, KInt, "")
	def(PSubLevel, "sub.lvl", "SUB LVL", "Sub Level", "SUB / NOISE / FM", 0, 1, 0, KLin, "%")
	def(PNoise, "noise", "NOISE", "Noise Level", "SUB / NOISE / FM", 0, 1, 0, KLin, "%")

	choice(PFType, "f.type", "TYPE", "Filter Type", "FILTER", FilterTypes, 0)
	def(PCutoff, "f.cut", "CUTOFF", "Filter Cutoff", "FILTER", 20, 20000, 2000, KExp, "Hz")
	def(PReso, "f.res", "RESO", "Filter Resonance", "FILTER", 0, 1, 0.2, KLin, "%")
	def(PFDrive, "f.drv", "DRIVE", "Filter Drive", "FILTER", 0, 1, 0, KLin, "%")
	def(PFEnv, "f.env", "ENV AMT", "Filter Env Amount", "FILTER", -1, 1, 0.3, KLin, "+-%")
	def(PFKey, "f.key", "KEY TRK", "Filter Key Tracking", "FILTER", 0, 1, 0.3, KLin, "%")

	envs := func(base PID, k, n string, a, d, s, r float64) {
		def(base, k+".a", "ATK", n+" Attack", n, 0.0005, 5, a, KExp, "s")
		def(base+1, k+".d", "DEC", n+" Decay", n, 0.005, 8, d, KExp, "s")
		def(base+2, k+".s", "SUS", n+" Sustain", n, 0, 1, s, KLin, "%")
		def(base+3, k+".r", "REL", n+" Release", n, 0.005, 8, r, KExp, "s")
	}
	envs(PAmpA, "amp", "AMP ENV", 0.002, 0.3, 0.8, 0.12)
	envs(PFltA, "flt", "FILTER ENV", 0.0005, 0.25, 0.3, 0.2)
	envs(PModA, "mod", "MOD ENV", 0.0005, 0.5, 0, 0.3)

	lfo := func(base PID, k, n string, rate float64, div int) {
		choice(base, k+".shape", "SHAPE", n+" Shape", n, LFOShapes, 0)
		choice(base+1, k+".sync", "SYNC", n+" Tempo Sync", n, []string{"Free", "BPM"}, 1)
		def(base+2, k+".rate", "RATE", n+" Rate (free)", n, 0.05, 40, rate, KExp, "Hz")
		choice(base+3, k+".div", "DIV", n+" Rate (BPM sync)", n, divNames(LFODivs), div)
		choice(base+4, k+".retrig", "RETRIG", n+" Retrigger", n, offOn, 0)
	}
	lfo(PL1Shape, "lfo1", "LFO 1", 4, 11)
	lfo(PL2Shape, "lfo2", "LFO 2", 0.5, 8)

	for i := 0; i < ModSlots; i++ {
		b := PM1Src + PID(3*i)
		k := fmt.Sprintf("m%d", i+1)
		choice(b, k+".src", "SRC", "", "MOD MATRIX", SrcNames, 0)
		choice(b+1, k+".dst", "DST", "", "MOD MATRIX", DstNames, 0)
		def(b+2, k+".amt", "AMT", fmt.Sprintf("Mod %d Amount", i+1), "MOD MATRIX", -1, 1, 0, KLin, "+-%")
		Metas[b].NoKnob, Metas[b+1].NoKnob = true, true
	}

	choice(PMode, "mode", "MODE", "Voice Mode", "VOICE", VoiceModes, 1)
	def(PGlide, "glide", "GLIDE", "Glide Time", "VOICE", 0, 2, 0, KSq, "s")
	def(PBend, "bend", "BEND", "Pitch Bend Range", "VOICE", 0, 24, 2, KInt, "st")
	def(PVelSens, "vel", "VEL", "Velocity Sensitivity", "VOICE", 0, 1, 0.5, KLin, "%")

	choice(PDistType, "dist.type", "DIST", "Distortion Type", "EFFECTS", DistTypes, 0)
	def(PDistDrive, "dist.drv", "DRIVE", "Distortion Drive", "EFFECTS", 0, 1, 0.4, KLin, "%")
	def(PDistMix, "dist.mix", "DIST MIX", "Distortion Mix", "EFFECTS", 0, 1, 0, KLin, "%")
	def(PCrush, "crush", "CRUSH", "Bit Crush", "EFFECTS", 0, 1, 0, KLin, "%")
	def(PChorus, "chorus", "CHORUS", "Chorus Mix", "EFFECTS", 0, 1, 0, KLin, "%")
	choice(PDlyDiv, "dly.div", "DLY TIME", "Delay Time", "EFFECTS", divNames(DelayDivs), 3)
	def(PDlyFB, "dly.fb", "DLY FB", "Delay Feedback", "EFFECTS", 0, 0.95, 0.35, KLin, "%")
	def(PDlyMix, "dly.mix", "DLY MIX", "Delay Mix", "EFFECTS", 0, 1, 0, KLin, "%")
	def(PRevSize, "rev.size", "REV SIZE", "Reverb Size", "EFFECTS", 0, 1, 0.5, KLin, "%")
	def(PRevMix, "rev.mix", "REV MIX", "Reverb Mix", "EFFECTS", 0, 1, 0, KLin, "%")
	def(PBass, "bass", "BASS +", "Bass Boost", "MASTER", 0, 12, 0, KLin, "dB")
	def(PWidth, "width", "WIDTH", "Stereo Width", "MASTER", 0, 2, 1, KLin, "%")
	def(PVolume, "vol", "VOLUME", "Master Volume", "MASTER", 0, 1, 0.7, KLin, "%")

	choice(PArpMode, "arp.mode", "MODE", "Arp / Repeat Mode", "ARP / NOTE REPEAT", ArpModes, 0)
	choice(PArpRate, "arp.rate", "RATE", "Arp / Repeat Rate", "ARP / NOTE REPEAT", divNames(ArpRates), 3)
	def(PArpOct, "arp.oct", "OCTAVES", "Arp Octaves", "ARP / NOTE REPEAT", 1, 4, 1, KInt, "")
	def(PArpGate, "arp.gate", "GATE", "Arp / Repeat Gate", "ARP / NOTE REPEAT", 0.05, 1.5, 0.6, KLin, "%")
	def(PArpSwing, "arp.swing", "SWING", "Arp / Repeat Swing", "ARP / NOTE REPEAT", 0, 0.6, 0, KLin, "%")
	choice(PArpLatch, "arp.latch", "LATCH", "Arp Latch", "ARP / NOTE REPEAT", offOn, 0)
	for i := PArpMode; i <= PArpLatch; i++ {
		Metas[i].Global = true
	}
	for i := range Metas {
		if Metas[i].Key == "" {
			panic(fmt.Sprintf("param %d undefined", i))
		}
	}
}

func divNames(d []Div) []string {
	s := make([]string, len(d))
	for i := range d {
		s[i] = d[i].Name
	}
	return s
}

// Stepped reports whether the parameter takes whole-number / named values.
func (m *Meta) Stepped() bool { return m.Kind == KInt || m.Kind == KChoice }

// FromNorm maps a 0..1 knob position to a parameter value.
func (m *Meta) FromNorm(n float64) float64 {
	n = math.Max(0, math.Min(1, n))
	switch m.Kind {
	case KExp:
		return m.Min * math.Pow(m.Max/m.Min, n)
	case KSq:
		return m.Min + n*n*(m.Max-m.Min)
	case KInt, KChoice:
		return math.Round(m.Min + n*(m.Max-m.Min))
	}
	return m.Min + n*(m.Max-m.Min)
}

// ToNorm maps a parameter value to a 0..1 knob position.
func (m *Meta) ToNorm(v float64) float64 {
	var n float64
	switch m.Kind {
	case KExp:
		n = math.Log(v/m.Min) / math.Log(m.Max/m.Min)
	case KSq:
		n = math.Sqrt(math.Max(0, (v-m.Min)/(m.Max-m.Min)))
	default:
		n = (v - m.Min) / (m.Max - m.Min)
	}
	return math.Max(0, math.Min(1, n))
}

// Clamp limits v to the parameter's range (and rounds stepped parameters).
func (m *Meta) Clamp(v float64) float64 {
	if m.Stepped() {
		v = math.Round(v)
	}
	return math.Max(m.Min, math.Min(m.Max, v))
}

// Format renders a value for display.
func (m *Meta) Format(v float64) string {
	switch m.Kind {
	case KChoice:
		i := int(math.Round(v))
		if i >= 0 && i < len(m.Choices) {
			return m.Choices[i]
		}
		return "?"
	case KInt:
		if m.Min < 0 && v > 0 {
			return fmt.Sprintf("+%d%s", int(v), m.Unit)
		}
		return fmt.Sprintf("%d%s", int(v), m.Unit)
	}
	switch m.Unit {
	case "%":
		return fmt.Sprintf("%d%%", int(math.Round(v*100)))
	case "+-%":
		return fmt.Sprintf("%+d%%", int(math.Round(v*100)))
	case "Hz":
		if v >= 1000 {
			return fmt.Sprintf("%.1fk", v/1000)
		}
		if v >= 100 {
			return fmt.Sprintf("%.0fHz", v)
		}
		return fmt.Sprintf("%.1fHz", v)
	case "s":
		if v < 1 {
			return fmt.Sprintf("%.0fms", v*1000)
		}
		return fmt.Sprintf("%.1fs", v)
	case "ct":
		if math.Abs(v) < 0.5 {
			return "0ct"
		}
		return fmt.Sprintf("%+.0fct", v)
	case "dB":
		return fmt.Sprintf("+%.1fdB", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// Patch is a full set of parameter values.
type Patch [NumParams]float64

// DefaultPatch returns every parameter at its default.
func DefaultPatch() Patch {
	var p Patch
	for i := range Metas {
		p[i] = Metas[i].Def
	}
	return p
}

// KeyToPID finds a parameter by its saved key.
func KeyToPID(key string) (PID, bool) {
	for i := range Metas {
		if Metas[i].Key == key {
			return PID(i), true
		}
	}
	return 0, false
}
