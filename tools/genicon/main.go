// Command genicon renders the Dr. Markdown, .MD 1024x1024 app icon using
// only the Go standard library: a medical-scrubs teal rounded-rectangle
// background with a white "M + stethoscope" mark centered on it. The mark
// is a bolder, simplified sibling of the banner mark in tools/genbanner so
// it survives small (64px) rendering: chunky Y-tubes with ear tips and an
// emphasized two-tone chestpiece.
//
// Usage: go run ./tools/genicon -out build/appicon.png
package main

import (
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 1024

var (
	teal  = color.RGBA{15, 118, 110, 255}
	white = color.RGBA{255, 255, 255, 255}
)

type pt struct{ x, y float64 }

// inPoly reports whether p is inside polygon pts (even-odd ray casting).
func inPoly(p pt, pts []pt) bool {
	inside := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		if (pts[i].y > p.y) != (pts[j].y > p.y) &&
			p.x < (pts[j].x-pts[i].x)*(p.y-pts[i].y)/(pts[j].y-pts[i].y)+pts[i].x {
			inside = !inside
		}
	}
	return inside
}

// fillPoly fills a polygon via bounding-box scan.
func fillPoly(img *image.RGBA, pts []pt, c color.RGBA) {
	minX, minY, maxX, maxY := pts[0].x, pts[0].y, pts[0].x, pts[0].y
	for _, p := range pts[1:] {
		minX, minY = math.Min(minX, p.x), math.Min(minY, p.y)
		maxX, maxY = math.Max(maxX, p.x), math.Max(maxY, p.y)
	}
	for y := int(minY); y <= int(maxY); y++ {
		for x := int(minX); x <= int(maxX); x++ {
			if inPoly(pt{float64(x) + 0.5, float64(y) + 0.5}, pts) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// thickLine returns a polygon for a stroke of width w from a to b.
func thickLine(a, b pt, w float64) []pt {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l*w/2, dx/l*w/2
	return []pt{
		{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny}, {a.x - nx, a.y - ny},
	}
}

// stroke draws a thick line with round-ish caps (a disc at each end).
func stroke(img *image.RGBA, a, b pt, w float64, c color.RGBA) {
	fillPoly(img, thickLine(a, b, w), c)
	for _, p := range []pt{a, b} {
		r := w / 2
		for y := int(p.y - r); y <= int(p.y+r); y++ {
			for x := int(p.x - r); x <= int(p.x+r); x++ {
				if math.Hypot(float64(x)+0.5-p.x, float64(y)+0.5-p.y) <= r {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
}

// disc fills a solid disc of radius r centered at p.
func disc(img *image.RGBA, p pt, r float64, c color.RGBA) {
	for y := int(p.y - r); y <= int(p.y+r); y++ {
		for x := int(p.x - r); x <= int(p.x+r); x++ {
			if math.Hypot(float64(x)+0.5-p.x, float64(y)+0.5-p.y) <= r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// roundedRect fills the full canvas with a rounded rectangle of radius r.
func roundedRect(img *image.RGBA, r float64, c color.RGBA) {
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	// Punch out the four corners to transparency.
	blank := color.RGBA{}
	corners := []pt{{0, 0}, {size, 0}, {0, size}, {size, size}}
	centers := []pt{{r, r}, {size - r, r}, {r, size - r}, {size - r, size - r}}
	for i, corner := range corners {
		for y := int(math.Min(corner.y, centers[i].y)); y <= int(math.Max(corner.y, centers[i].y)); y++ {
			for x := int(math.Min(corner.x, centers[i].x)); x <= int(math.Max(corner.x, centers[i].x)); x++ {
				if math.Hypot(float64(x)+0.5-centers[i].x, float64(y)+0.5-centers[i].y) > r {
					img.SetRGBA(x, y, blank)
				}
			}
		}
	}
}

func main() {
	out := flag.String("out", "build/appicon.png", "output PNG path")
	flag.Parse()

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	roundedRect(img, 200, teal)

	// The mark is designed in the coordinate space below, then scaled by k
	// about the canvas center and shifted so its bounding box is centered.
	const k = 1.0
	const ox, oy = -52.0, -11.0
	P := func(x, y float64) pt {
		return pt{512 + (x-512)*k + ox, 512 + (y-512)*k + oy}
	}

	// "M" — four bold strokes.
	const mw = 84.0
	stroke(img, P(240, 740), P(240, 300), mw*k, white)
	stroke(img, P(240, 300), P(380, 500), mw*k, white)
	stroke(img, P(380, 500), P(520, 300), mw*k, white)
	stroke(img, P(520, 300), P(520, 740), mw*k, white)
	// Stethoscope — Y-tubes with ear tips, single tube arcing right into a
	// large two-tone chestpiece.
	const tw = 64.0
	stroke(img, P(648, 330), P(716, 486), tw*k, white)
	stroke(img, P(784, 330), P(716, 486), tw*k, white)
	disc(img, P(648, 318), 48*k, white)
	disc(img, P(784, 318), 48*k, white)
	stroke(img, P(716, 486), P(712, 566), tw*k, white)
	stroke(img, P(712, 566), P(722, 636), tw*k, white)
	stroke(img, P(722, 636), P(752, 690), tw*k, white)
	stroke(img, P(752, 690), P(800, 714), tw*k, white)
	disc(img, P(846, 702), 84*k, white)
	disc(img, P(846, 702), 40*k, teal)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
