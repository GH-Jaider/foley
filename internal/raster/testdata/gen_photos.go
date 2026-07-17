//go:build ignore

// gen_photos regenerates the photo-*.png fixtures deterministically —
// the same provenance discipline as scripts/fonts.sh: every binary
// asset in the repo must be re-derivable from committed code. Run from
// this directory:
//
//	go run gen_photos.go
//
// The images are "photo-like" on purpose (smooth gradients, curves —
// nothing flat like the checkerboard fixtures) so placement scaling is
// exercised on realistic content. photo-glass.png carries REAL alpha
// (transparent → translucent gradient): the fixture for straight-vs-
// premultiplied regression tests, the one axis opaque fixtures cannot
// see.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

func main() {
	write("photo-sunset.png", 64, 40, sunset)
	write("photo-leaf.png", 48, 48, leaf)
	write("photo-glass.png", 32, 32, glass)
}

func write(name string, w, h int, px func(x, y int) color.NRGBA) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, px(x, y))
		}
	}
	f, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%dx%d)", name, w, h)
}

func clamp(v int) uint8 {
	return uint8(max(0, min(255, v)))
}

// sunset: sky gradient + sun disc + dark hills.
func sunset(x, y int) color.NRGBA {
	const H = 40.0
	t := float64(y) / H
	r, g, b := int(250-160*t), int(140-90*t), int(90+60*t)
	dx, dy := x-44, y-14
	if dx*dx+dy*dy <= 36 {
		r, g, b = 255, 236, 170
	}
	hill := 30 + 6*math.Sin(float64(x)/7.0)
	if float64(y) > hill {
		shade := int(18 + 10*(float64(y)-hill)/H)
		r, g, b = shade, shade+8, shade+4
	}
	return color.NRGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 0xff}
}

// leaf: green radial gradient + veins.
func leaf(x, y int) color.NRGBA {
	const cx, cy = 24.0, 24.0
	fx, fy := float64(x), float64(y)
	d := math.Sqrt((fx-cx)*(fx-cx)+(fy-cy)*(fy-cy)) / 24.0
	g := int(200 - 130*math.Min(1, d))
	r, b := int(40+30*d), int(50+20*d)
	if math.Abs((fx-cx)-(fy-cy)) < 2 || math.Abs(fx-cx) < 1 {
		r, g, b = 30, 90, 45
	}
	return color.NRGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 0xff}
}

// glass: a red pane whose STRAIGHT alpha ramps 0→255 left to right, with
// a solid half-alpha band in the middle rows — exact known values for
// blending asserts.
func glass(x, y int) color.NRGBA {
	a := clamp(x * 255 / 31)
	if y >= 12 && y < 20 {
		a = 128
	}
	return color.NRGBA{R: 0xff, G: 0x00, B: 0x00, A: a}
}
