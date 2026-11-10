// Command shot renders the WaveBass window to a PNG without a display, to check the layout.
//
//	go run ./tools/shot out.png [preset name]
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/codypowpow-afk/wavebass/internal/ui"
	"github.com/codypowpow-afk/wavebass/synth"
)

const ss = 2 // supersampling

type op func()

type soft struct {
	img                  *image.RGBA
	rects, shapes, texts []op
	faces                [6]font.Face
}

func (s *soft) px(x, y int, c color.RGBA, a float64) {
	if x < 0 || y < 0 || x >= s.img.Rect.Dx() || y >= s.img.Rect.Dy() || a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	o := s.img.RGBAAt(x, y)
	mix := func(p, q uint8) uint8 { return uint8(float64(p)*(1-a) + float64(q)*a) }
	s.img.SetRGBA(x, y, color.RGBA{mix(o.R, c.R), mix(o.G, c.G), mix(o.B, c.B), 255})
}

func (s *soft) Rect(x, y, w, h float32, c color.RGBA) {
	s.rects = append(s.rects, func() {
		for j := int(y * ss); j < int((y+h)*ss); j++ {
			for i := int(x * ss); i < int((x+w)*ss); i++ {
				s.px(i, j, c, 1)
			}
		}
	})
}

func (s *soft) Circle(x, y, r float32, c color.RGBA) {
	s.shapes = append(s.shapes, func() {
		for j := int((y - r - 1) * ss); j <= int((y+r+1)*ss); j++ {
			for i := int((x - r - 1) * ss); i <= int((x+r+1)*ss); i++ {
				d := math.Hypot(float64(i)/ss-float64(x), float64(j)/ss-float64(y))
				s.px(i, j, c, (float64(r)-d)*ss+0.5)
			}
		}
	})
}

func (s *soft) seg(x0, y0, x1, y1, w float32, c color.RGBA) {
	minx, maxx := math.Min(float64(x0), float64(x1))-float64(w), math.Max(float64(x0), float64(x1))+float64(w)
	miny, maxy := math.Min(float64(y0), float64(y1))-float64(w), math.Max(float64(y0), float64(y1))+float64(w)
	dx, dy := float64(x1-x0), float64(y1-y0)
	l2 := dx*dx + dy*dy
	for j := int(miny * ss); j <= int(maxy*ss); j++ {
		for i := int(minx * ss); i <= int(maxx*ss); i++ {
			px, py := float64(i)/ss, float64(j)/ss
			t := 0.0
			if l2 > 0 {
				t = math.Max(0, math.Min(1, ((px-float64(x0))*dx+(py-float64(y0))*dy)/l2))
			}
			d := math.Hypot(px-(float64(x0)+t*dx), py-(float64(y0)+t*dy))
			s.px(i, j, c, (float64(w)/2-d)*ss+0.5)
		}
	}
}

func (s *soft) Arc(cx, cy, r, a0, a1, width float32, c color.RGBA) {
	s.shapes = append(s.shapes, func() {
		n := int((a1-a0)*r/2) + 2
		for i := 0; i < n; i++ {
			t0 := float64(a0) + float64(a1-a0)*float64(i)/float64(n)
			t1 := float64(a0) + float64(a1-a0)*float64(i+1)/float64(n)
			s.seg(cx+r*float32(math.Cos(t0)), cy+r*float32(math.Sin(t0)), cx+r*float32(math.Cos(t1)), cy+r*float32(math.Sin(t1)), width, c)
		}
	})
}

func (s *soft) Polyline(pts []float32, width float32, c color.RGBA) {
	p := append([]float32(nil), pts...)
	s.shapes = append(s.shapes, func() {
		for i := 0; i+3 < len(p); i += 2 {
			s.seg(p[i], p[i+1], p[i+2], p[i+3], width, c)
		}
	})
}

func (s *soft) Text(str string, f ui.Font, x, y float32, c color.RGBA, align int) {
	s.texts = append(s.texts, func() {
		face := s.faces[f]
		d := font.Drawer{Dst: s.img, Src: image.NewUniform(c), Face: face}
		w := d.MeasureString(str)
		px := fixed.I(int(x * ss))
		switch align {
		case ui.AlignC:
			px -= w / 2
		case ui.AlignR:
			px -= w
		}
		d.Dot = fixed.Point26_6{X: px, Y: fixed.I(int(y*ss)) + face.Metrics().Ascent}
		d.DrawString(str)
	})
}

func (s *soft) Layer() {
	for _, l := range [][]op{s.rects, s.shapes, s.texts} {
		for _, f := range l {
			f()
		}
	}
	s.rects, s.shapes, s.texts = nil, nil, nil
}

func main() {
	out := "shot.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	cfg := ui.DefaultConfig()
	if len(os.Args) > 2 {
		cfg.Last = os.Args[2]
	}
	eng := synth.NewEngine()
	app := ui.New(eng, cfg)
	app.Devices = func() ([]string, []string) { return []string{"MPK mini 3"}, nil }
	// Pretend a keyboard is plugged in: turn four knobs, hold a note.
	for i, cc := range []byte{70, 71, 72, 73} {
		app.HandleMIDI(0xB0, cc, byte(40+i*20))
	}
	app.HandleMIDI(0x90, 60, 100)
	eng.Process(make([]float32, 2*synth.SampleRate/3))
	app.Frame()

	s := &soft{img: image.NewRGBA(image.Rect(0, 0, ui.W*ss, ui.H*ss))}
	reg, _ := opentype.Parse(goregular.TTF)
	bold, _ := opentype.Parse(gobold.TTF)
	mk := func(f *opentype.Font, size float64) font.Face {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size * ss, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			log.Fatal(err)
		}
		return face
	}
	s.faces = [6]font.Face{mk(bold, 9), mk(bold, 11), mk(reg, 13), mk(bold, 14), mk(bold, 18), mk(bold, 24)}
	app.Draw(s)
	s.Layer()
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, s.img); err != nil {
		log.Fatal(err)
	}
}
