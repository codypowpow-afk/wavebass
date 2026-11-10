package synth

import (
	"math"
	"math/cmplx"
	"math/rand"
	"sync"
)

// Wavetables: every table is a row of Frames single-cycle waves that the oscillator morphs
// between. Each frame is stored band-limited at several octaves (mip levels) so high notes
// don't alias.
const (
	TableSize = 2048
	Frames    = 16
	Mips      = 10
	maxHarm   = TableSize/2 - 1
)

// Table indices (the order of tableDefs).
const (
	TBasic = iota
	TSawFilter
	TPWM
	TSyncSaw
	TSyncSine
	TFold
	TFM1
	TFM2
	TFM3
	TVowel
	TGrowl
	TYoy
	TReso
	TOrgan
	THollow
	TDigital
	TMetal
	TCrush
	TScreech
	TPhaseDist
	TSaturate
	TOctaves
	TComb
	TRectify
	THarmonics
	TNeuro
	TFoldSaw
	NumTables
)

type tableDef struct {
	name string
	time func(t float64, w []float64)       // fills one cycle, or
	spec func(t float64, amp, ph []float64) // fills harmonic amplitudes / phases (index = harmonic)
}

// Table holds the rendered frames. mip[k][frame] has TableSize+1 samples (the last repeats the
// first so interpolation needs no wrap).
type Table struct {
	Name string
	mip  [Mips][Frames][]float32
}

// Frame returns the full-band wave for display.
func (t *Table) Frame(f int) []float32 { return t.mip[2][f] }

var (
	tables     []*Table
	tablesOnce sync.Once
)

// Tables builds (once) and returns every wavetable.
func Tables() []*Table {
	tablesOnce.Do(func() {
		tables = make([]*Table, len(tableDefs))
		var wg sync.WaitGroup
		for i := range tableDefs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				tables[i] = buildTable(&tableDefs[i])
			}(i)
		}
		wg.Wait()
	})
	return tables
}

func fft(a []complex128, inverse bool) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for l := 2; l <= n; l <<= 1 {
		ang := 2 * math.Pi / float64(l)
		if !inverse {
			ang = -ang
		}
		wl := cmplx.Rect(1, ang)
		for i := 0; i < n; i += l {
			w := complex(1, 0)
			for j := 0; j < l/2; j++ {
				u, v := a[i+j], a[i+j+l/2]*w
				a[i+j], a[i+j+l/2] = u+v, u-v
				w *= wl
			}
		}
	}
}

func buildTable(d *tableDef) *Table {
	t := &Table{Name: d.name}
	w := make([]float64, TableSize)
	amp := make([]float64, maxHarm+1)
	ph := make([]float64, maxHarm+1)
	c := make([]complex128, maxHarm+1) // c[h]: x[n] = sum 2*Re(c[h] e^{i 2 pi h n / N})
	buf := make([]complex128, TableSize)
	for f := 0; f < Frames; f++ {
		pos := float64(f) / float64(Frames-1)
		if d.spec != nil {
			for i := range amp {
				amp[i], ph[i] = 0, 0
			}
			d.spec(pos, amp, ph)
			for h := 1; h <= maxHarm; h++ {
				c[h] = cmplx.Rect(amp[h]/2, ph[h]-math.Pi/2)
			}
		} else {
			d.time(pos, w)
			for i := range buf {
				buf[i] = complex(w[i], 0)
			}
			fft(buf, false)
			for h := 1; h <= maxHarm; h++ {
				c[h] = buf[h] / complex(TableSize, 0)
			}
		}
		// Normalise the loudness of every frame.
		render := func(limit int) {
			for i := range buf {
				buf[i] = 0
			}
			for h := 1; h <= limit && h <= maxHarm; h++ {
				buf[h] = c[h]
				buf[TableSize-h] = cmplx.Conj(c[h])
			}
			fft(buf, true)
		}
		render(maxHarm)
		var sum, peak float64
		for i := range buf {
			v := real(buf[i])
			sum += v * v
			peak = math.Max(peak, math.Abs(v))
		}
		rms := math.Sqrt(sum / TableSize)
		scale := 1.0
		if rms > 1e-9 {
			scale = math.Min(0.42/rms, 1.5/peak)
		}
		for k := 0; k < Mips; k++ {
			render((TableSize / 2) >> k)
			out := make([]float32, TableSize+1)
			for i := 0; i < TableSize; i++ {
				out[i] = float32(real(buf[i]) * scale)
			}
			out[TableSize] = out[0]
			t.mip[k][f] = out
		}
	}
	return t
}

// mipFor picks the mip level whose harmonics all stay below Nyquist at frequency f.
func mipFor(f float64) int {
	r := f * (TableSize / 2) / (SampleRate * 0.5)
	k := 0
	for r > 1 && k < Mips-1 {
		r *= 0.5
		k++
	}
	return k
}

// ---- helpers for the table definitions ----

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func frac(x float64) float64 { return x - math.Floor(x) }

func timeShape(fn func(t, x float64) float64) func(float64, []float64) {
	return func(t float64, w []float64) {
		for i := range w {
			w[i] = fn(t, (float64(i)+0.5)/TableSize)
		}
	}
}

func sawAt(x float64) float64 { return 1 - 2*frac(x) }
func sqAt(x float64) float64 {
	if frac(x) < 0.5 {
		return 1
	}
	return -1
}
func triAt(x float64) float64 {
	x = frac(x + 0.25)
	return 1 - 4*math.Abs(x-0.5)
}
func sinAt(x float64) float64 { return math.Sin(2 * math.Pi * x) }

func gauss(x, c, w float64) float64 {
	d := (x - c) / w
	return math.Exp(-d * d)
}

// segment splits t (0..1) into n equal segments: returns segment index and position inside it.
func segment(t float64, n int) (int, float64) {
	p := t * float64(n)
	i := int(p)
	if i >= n {
		i = n - 1
	}
	return i, p - float64(i)
}

const formantF0 = 70.0 // reference pitch for vowel tables: harmonic h sits at h*70 Hz

func formants(amp []float64, fs [3]float64, gains [3]float64, bw float64, base float64) {
	for h := 1; h <= 200; h++ {
		f := float64(h) * formantF0
		a := base / float64(h)
		for i := range fs {
			a += gains[i] * gauss(f, fs[i], bw*(1+0.4*float64(i)))
		}
		amp[h] = a
	}
	amp[1] += 0.6
}

var vowelF = [][3]float64{
	{730, 1090, 2440}, // A
	{530, 1840, 2480}, // E
	{270, 2290, 3010}, // I
	{570, 840, 2410},  // O
	{300, 870, 2240},  // U
}

func lerp3(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)}
}

var tableDefs = []tableDef{
	{name: "Basic Shapes", time: timeShape(func(t, x float64) float64 {
		// sine → triangle → saw → square → thin pulse
		s, f := segment(t, 4)
		pulse := func(x float64) float64 {
			if frac(x) < 0.1 {
				return 1
			}
			return -1
		}
		shapes := []func(float64) float64{sinAt, triAt, sawAt, sqAt, pulse}
		return lerp(shapes[s](x), shapes[s+1](x), f)
	})},
	{name: "Saw Filter", spec: func(t float64, amp, ph []float64) {
		hc := math.Pow(2, 0.3+t*8.2)
		for h := 1; h <= maxHarm; h++ {
			r := float64(h) / hc
			amp[h] = (1/float64(h))/math.Sqrt(1+r*r*r*r) + 0.5*gauss(float64(h), hc, 0.35*hc+0.5)/math.Sqrt(hc)
		}
	}},
	{name: "PWM", time: timeShape(func(t, x float64) float64 {
		if x < lerp(0.5, 0.04, t) {
			return 1
		}
		return -1
	})},
	{name: "Sync Saw", time: timeShape(func(t, x float64) float64 {
		return sawAt(x * (1 + t*7))
	})},
	{name: "Sync Sine", time: timeShape(func(t, x float64) float64 {
		return sinAt(x*(1+t*11)) * (1 - x) * (1 - x)
	})},
	{name: "Wavefold", time: timeShape(func(t, x float64) float64 {
		return math.Sin(math.Pi / 2 * (1 + t*8) * sinAt(x))
	})},
	{name: "FM 1:1", time: timeShape(func(t, x float64) float64 {
		return math.Sin(2*math.Pi*x + t*6*sinAt(x))
	})},
	{name: "FM 2:1", time: timeShape(func(t, x float64) float64 {
		return math.Sin(2*math.Pi*x + t*5*sinAt(2*x))
	})},
	{name: "FM 3:1", time: timeShape(func(t, x float64) float64 {
		return math.Sin(2*math.Pi*x + t*4.5*sinAt(3*x) + t*t*1.5*sinAt(x))
	})},
	{name: "Vowels AEIOU", spec: func(t float64, amp, ph []float64) {
		s, f := segment(t, 4)
		formants(amp, lerp3(vowelF[s], vowelF[s+1], f), [3]float64{1, 0.6, 0.3}, 90, 0.12)
	}},
	{name: "Growl", spec: func(t float64, amp, ph []float64) {
		u := 0.5 - 0.5*math.Cos(math.Pi*t)
		f1 := lerp(260, 1150, u)
		f2 := lerp(2600, 900, t*t)
		formants(amp, [3]float64{f1, f2, 3400 - 900*t}, [3]float64{1.2, 0.8, 0.35}, 140, 0.3)
		r := rand.New(rand.NewSource(11))
		for h := 2; h <= 200; h++ {
			ph[h] = r.Float64() * 2.2
		}
	}},
	{name: "Yoy", spec: func(t float64, amp, ph []float64) {
		// "oh" → "ah" → "ee"
		s, f := segment(t, 2)
		pts := [][3]float64{{380, 760, 2500}, {820, 1250, 2600}, {290, 2500, 3200}}
		formants(amp, lerp3(pts[s], pts[s+1], f), [3]float64{1.1, 0.9, 0.3}, 110, 0.08)
	}},
	{name: "Reso Sweep", spec: func(t float64, amp, ph []float64) {
		hc := 1.5 * math.Pow(40, t)
		for h := 1; h <= maxHarm; h++ {
			r := float64(h) / hc
			amp[h] = (1/float64(h))/(1+r*r) + 1.4*gauss(float64(h), hc, 0.12*hc+0.6)
		}
	}},
	{name: "Organ", spec: func(t float64, amp, ph []float64) {
		bars := []int{1, 2, 3, 4, 6, 8, 10, 12, 16}
		regs := [][]float64{
			{1, 0, 0, 0, 0, 0, 0, 0, 0},
			{1, 0.8, 0.5, 0, 0, 0, 0, 0, 0},
			{0.8, 1, 0.3, 0.8, 0.2, 0.5, 0, 0, 0},
			{1, 0.6, 0.8, 0.9, 0.7, 0.8, 0.5, 0.6, 0.7},
		}
		s, f := segment(t, 3)
		for i, h := range bars {
			amp[h] = lerp(regs[s][i], regs[s+1][i], f)
			ph[h] = float64(i) * 0.7
		}
	}},
	{name: "Hollow", spec: func(t float64, amp, ph []float64) {
		for h := 1; h <= maxHarm; h += 2 {
			a := 1 / float64(h)
			if h%4 == 3 {
				a *= 1 - t
			}
			amp[h] = a * (1 + t*1.5*gauss(float64(h), 9, 4))
		}
	}},
	{name: "Digital", spec: func(t float64, amp, ph []float64) {
		r := rand.New(rand.NewSource(5))
		var sp [6][65]float64
		for i := range sp {
			for h := 1; h <= 64; h++ {
				v := r.Float64()
				sp[i][h] = v * v * v / math.Pow(float64(h), 0.7)
			}
			sp[i][1] += 0.5
		}
		s, f := segment(t, 5)
		for h := 1; h <= 64; h++ {
			amp[h] = lerp(sp[s][h], sp[s+1][h], f)
			ph[h] = float64(h*h) * 0.37
		}
	}},
	{name: "Metal", spec: func(t float64, amp, ph []float64) {
		e := 1 + t*1.1
		for k := 1; k <= 40; k++ {
			hf := math.Pow(float64(k), e)
			h := int(hf)
			if h+1 > maxHarm {
				break
			}
			a := 1 / math.Pow(float64(k), 0.8)
			amp[h] += a * (1 - (hf - float64(h)))
			amp[h+1] += a * (hf - float64(h))
		}
	}},
	{name: "Bit Crush", time: timeShape(func(t, x float64) float64 {
		steps := math.Round(lerp(40, 3, math.Sqrt(t)))
		xs := math.Round(lerp(128, 6, t))
		xq := math.Floor(x*xs) / xs
		return math.Round(lerp(sawAt(xq), sinAt(xq), 0.5)*steps) / steps
	})},
	{name: "Screech", spec: func(t float64, amp, ph []float64) {
		hc := lerp(5, 46, t)
		for h := 1; h <= 120; h++ {
			amp[h] = gauss(float64(h), hc, 2.2+t*3) + 0.6*gauss(float64(h), hc*1.6, 2) + 0.15/float64(h)
		}
		amp[1] += 0.5
	}},
	{name: "Phase Dist", time: timeShape(func(t, x float64) float64 {
		d := lerp(0.5, 0.03, t)
		var p float64
		if x < d {
			p = x / (2 * d)
		} else {
			p = 0.5 + (x-d)/(2*(1-d))
		}
		return -math.Cos(2 * math.Pi * p)
	})},
	{name: "808 Drive", time: timeShape(func(t, x float64) float64 {
		return math.Tanh((1 + t*t*24) * (sinAt(x) + 0.3*t*sinAt(2*x+0.2)))
	})},
	{name: "Octaves", time: timeShape(func(t, x float64) float64 {
		v := sawAt(x)
		for k := 1; k <= 4; k++ {
			w := math.Max(0, math.Min(1, t*4-float64(k-1)))
			v += w * sawAt(x*math.Pow(2, float64(k))) / float64(k)
		}
		return v
	})},
	{name: "Comb", spec: func(t float64, amp, ph []float64) {
		d := lerp(0.03, 0.5, t)
		for h := 1; h <= maxHarm; h++ {
			amp[h] = math.Abs(math.Cos(math.Pi*float64(h)*d)) / float64(h)
		}
		amp[1] += 0.3
	}},
	{name: "Rectify", time: timeShape(func(t, x float64) float64 {
		s := sinAt(x)
		seg, f := segment(t, 3)
		half := math.Max(s, 0)*2 - 0.64
		full := math.Abs(s)*2 - 1.27
		fold := math.Sin(math.Pi * 2.5 * math.Abs(s))
		sh := []float64{s, half, full, fold}
		return lerp(sh[seg], sh[seg+1], f)
	})},
	{name: "Harmonics", spec: func(t float64, amp, ph []float64) {
		p := 1 + t*11
		for h := 1; h <= 16; h++ {
			amp[h] = math.Max(0, 1-math.Abs(float64(h)-p))
		}
		amp[1] += 0.35
	}},
	{name: "Neuro", spec: func(t float64, amp, ph []float64) {
		n1 := lerp(3, 28, t)
		n2 := lerp(40, 9, t)
		pk := lerp(6, 34, t*t)
		for h := 1; h <= maxHarm; h++ {
			x := float64(h)
			a := 1 / x
			a *= 1 - 0.95*gauss(x, n1, 1.5+0.1*n1)
			a *= 1 - 0.95*gauss(x, n2, 2.5)
			a += 0.5 * gauss(x, pk, 1.8)
			amp[h] = a
			ph[h] = t * x * x * 0.035
		}
	}},
	{name: "Fold Saw", time: timeShape(func(t, x float64) float64 {
		return math.Sin(math.Pi / 2 * (1 + t*6) * lerp(sawAt(x), sqAt(x), t*0.5))
	})},
}
