package synth

import "math"

// Effects chain: distortion → bit crush → bass boost → chorus → delay → reverb → width →
// volume → limiter.

type comb struct {
	buf    []float64
	i      int
	filter float64
}

func (c *comb) tick(x, fb, damp float64) float64 {
	y := c.buf[c.i]
	c.filter = y*(1-damp) + c.filter*damp
	c.buf[c.i] = x + c.filter*fb
	c.i++
	if c.i >= len(c.buf) {
		c.i = 0
	}
	return y
}

type allpass struct {
	buf []float64
	i   int
}

func (a *allpass) tick(x float64) float64 {
	b := a.buf[a.i]
	y := b - x
	a.buf[a.i] = x + b*0.5
	a.i++
	if a.i >= len(a.buf) {
		a.i = 0
	}
	return y
}

type fx struct {
	dcL, dcR, dcXL, dcXR float64
	crL, crR, crPh       float64
	shL, shR             float64

	chBuf [2][]float64
	chW   int
	chPh  float64

	dBuf       [2][]float64
	dW         int
	dLen       float64
	dLpL, dLpR float64
	dHpL, dHpR float64

	combs [2][4]comb
	aps   [2][2]allpass
	rvHp  float64

	sideLp float64
	limEnv float64
	volSm  float64
}

func (f *fx) init() {
	for c := 0; c < 2; c++ {
		f.chBuf[c] = make([]float64, 4096)
		f.dBuf[c] = make([]float64, SampleRate*3)
		for i, n := range []int{1557, 1617, 1491, 1422} {
			f.combs[c][i].buf = make([]float64, (n+c*23)*SampleRate/44100)
		}
		for i, n := range []int{556, 441} {
			f.aps[c][i].buf = make([]float64, (n+c*23)*SampleRate/44100)
		}
	}
	f.dLen = SampleRate * 0.25
}

func distort(x float64, typ int, g float64) float64 {
	x *= g
	switch typ {
	case 1: // hard clip
		return clamp(x, -1, 1)
	case 2: // triangle wavefold
		x = 0.25*x + 0.25
		return 4*math.Abs(x-math.Floor(x)-0.5) - 1
	case 3: // fuzz: asymmetric
		if x > 0 {
			return tanhf(x * 2)
		}
		return tanhf(x*0.6) * 0.8
	case 4: // sine shaper
		return math.Sin(x * math.Pi / 2)
	case 5: // rectify (octave up)
		return tanhf(math.Abs(x)*1.5)*1.6 - 0.8
	}
	return tanhf(x)
}

func (f *fx) process(e *Engine) {
	S := &e.S
	distMix := S[PDistMix]
	dd := clamp(S[PDistDrive]+e.distMod, 0, 1)
	distG := 1 + dd*dd*40
	distComp := 1 - 0.45*dd
	distType := int(S[PDistType])
	crush := S[PCrush]
	crStep := 1 / (1 + crush*crush*40)
	crLevels := math.Pow(2, 15-crush*12.5)
	bassG := math.Pow(10, S[PBass]/20) - 1
	chMix := S[PChorus]
	dlyMix, dlyFB := S[PDlyMix], S[PDlyFB]
	dTarget := clamp(DelayDivs[int(S[PDlyDiv])].Beats*60/e.bpm*SampleRate, 64, float64(len(f.dBuf[0])-8))
	revMix := S[PRevMix]
	revFB := 0.72 + 0.24*S[PRevSize]
	width := S[PWidth]
	vol := S[PVolume] * S[PVolume] * 2.6
	const shelfC = 0.013 // ~100 Hz one-pole
	const sideC = 0.0195 // ~150 Hz

	for i := 0; i < cb; i++ {
		l, r := e.mixL[i]*0.5, e.mixR[i]*0.5

		if distMix > 0.001 {
			dl := distort(l, distType, distG) * distComp
			dr := distort(r, distType, distG) * distComp
			// DC blocker (asymmetric shapes add an offset)
			f.dcL = dl - f.dcXL + 0.9985*f.dcL
			f.dcXL = dl
			f.dcR = dr - f.dcXR + 0.9985*f.dcR
			f.dcXR = dr
			l += (f.dcL - l) * distMix
			r += (f.dcR - r) * distMix
		}

		if crush > 0.001 {
			f.crPh += crStep
			if f.crPh >= 1 {
				f.crPh -= 1
				f.crL = math.Round(l*crLevels) / crLevels
				f.crR = math.Round(r*crLevels) / crLevels
			}
			l, r = f.crL, f.crR
		}

		f.shL += (l - f.shL) * shelfC
		f.shR += (r - f.shR) * shelfC
		l += f.shL * bassG
		r += f.shR * bassG

		if chMix > 0.001 {
			f.chBuf[0][f.chW] = l
			f.chBuf[1][f.chW] = r
			f.chPh += 0.55 / SampleRate
			if f.chPh >= 1 {
				f.chPh -= 1
			}
			for c := 0; c < 2; c++ {
				ph := f.chPh + 0.25*float64(c)
				d := (0.012 + 0.004*math.Sin(2*math.Pi*ph)) * SampleRate
				p := float64(f.chW) - d
				if p < 0 {
					p += 4096
				}
				ip := int(p)
				fr := p - float64(ip)
				s := f.chBuf[c][ip&4095]*(1-fr) + f.chBuf[c][(ip+1)&4095]*fr
				d2 := (0.019 + 0.005*math.Sin(2*math.Pi*(ph+0.4))) * SampleRate
				p = float64(f.chW) - d2
				if p < 0 {
					p += 4096
				}
				ip = int(p)
				fr = p - float64(ip)
				s += f.chBuf[1-c][ip&4095]*(1-fr) + f.chBuf[1-c][(ip+1)&4095]*fr
				if c == 0 {
					l += s * 0.6 * chMix
				} else {
					r += s * 0.6 * chMix
				}
			}
			f.chW = (f.chW + 1) & 4095
		}

		if dlyMix > 0.001 {
			f.dLen += (dTarget - f.dLen) * 0.0004
			n := len(f.dBuf[0])
			p := float64(f.dW) - f.dLen
			if p < 0 {
				p += float64(n)
			}
			ip := int(p)
			fr := p - float64(ip)
			i1 := ip + 1
			if i1 >= n {
				i1 = 0
			}
			wl := f.dBuf[0][ip]*(1-fr) + f.dBuf[0][i1]*fr
			wr := f.dBuf[1][ip]*(1-fr) + f.dBuf[1][i1]*fr
			// feedback path: band-limited, cross-fed for ping-pong
			f.dLpL += (wl - f.dLpL) * 0.45
			f.dLpR += (wr - f.dLpR) * 0.45
			f.dHpL += (f.dLpL - f.dHpL) * 0.02
			f.dHpR += (f.dLpR - f.dHpR) * 0.02
			fl, fr2 := f.dLpL-f.dHpL, f.dLpR-f.dHpR
			f.dBuf[0][f.dW] = l + fr2*dlyFB
			f.dBuf[1][f.dW] = r*0.6 + fl*dlyFB
			f.dW++
			if f.dW >= n {
				f.dW = 0
			}
			l += wl * dlyMix
			r += wr * dlyMix
		} else if f.dLpL != 0 || f.dW != 0 {
			f.dLen = dTarget
		}

		if revMix > 0.001 {
			in := (l + r) * 0.5
			f.rvHp += (in - f.rvHp) * 0.026
			in = (in - f.rvHp) * 0.3
			var ol, or float64
			for k := 0; k < 4; k++ {
				ol += f.combs[0][k].tick(in, revFB, 0.3)
				or += f.combs[1][k].tick(in, revFB, 0.3)
			}
			for k := 0; k < 2; k++ {
				ol = f.aps[0][k].tick(ol)
				or = f.aps[1][k].tick(or)
			}
			l += ol * revMix
			r += or * revMix
		}

		// Width: the low end always stays mono.
		m, s := (l+r)*0.5, (l-r)*0.5
		f.sideLp += (s - f.sideLp) * sideC
		s = (s - f.sideLp) * width
		l, r = m+s, m-s

		f.volSm += (vol - f.volSm) * 0.002
		l *= f.volSm
		r *= f.volSm

		// Limiter.
		pk := math.Max(math.Abs(l), math.Abs(r))
		if pk > f.limEnv {
			f.limEnv = pk
		} else {
			f.limEnv *= 0.9997
		}
		if f.limEnv > 0.92 {
			gn := 0.92 / f.limEnv
			l *= gn
			r *= gn
		}
		if l != l || r != r {
			l, r = 0, 0
			f.limEnv = 0
		}
		e.outL[i], e.outR[i] = clamp(l, -1, 1), clamp(r, -1, 1)
	}
}
