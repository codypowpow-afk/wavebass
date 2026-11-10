// Package appicon draws the WaveBass icon.
package appicon

import (
	"image"
	"image/color"
	"math"
)

// Image draws the app icon (an orange wave morphing from sine to saw) at the given size.
func Image(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	wave := func(x float64) float64 {
		t := x
		sine := math.Sin(2 * math.Pi * x * 1.5)
		p := x * 1.5
		saw := 1 - 2*(p-math.Floor(p))
		return sine*(1-t) + saw*t
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := (float64(x)+0.5)/s, (float64(y)+0.5)/s
			c := color.RGBA{0x15, 0x16, 0x1a, 0xff}
			if fx > 0.08 && fx < 0.92 {
				u := (fx - 0.08) / 0.84
				wy := 0.5 - 0.3*wave(u)
				// thickness follows the slope so steep edges stay visible
				d := math.Abs(wave(u+0.5/s)-wave(u-0.5/s)) * 0.3
				if math.Abs(fy-wy) < 0.055+d {
					c = color.RGBA{0xff, 0x7a, 0x1a, 0xff}
				}
			}
			if fy > 0.86 && fy < 0.92 && fx > 0.08 && fx < 0.92 {
				c = color.RGBA{0x35, 0xd0, 0xc0, 0xff}
			}
			img.Set(x, y, c)
		}
	}
	return img
}
