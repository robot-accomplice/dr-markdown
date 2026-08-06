// Command genicon renders the Dr. Markdown 1024x1024 app icon using only
// the Go standard library: an indigo rounded-rectangle background with a
// white "M-down-arrow" markdown glyph centered on it.
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
	indigo = color.RGBA{79, 70, 229, 255}
	white  = color.RGBA{255, 255, 255, 255}
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
	roundedRect(img, 200, indigo)

	const w = 72.0 // stroke width
	// "M" — four strokes, occupying x 200..560, y 320..704.
	stroke(img, pt{210, 704}, pt{210, 330}, w, white)
	stroke(img, pt{210, 330}, pt{385, 560}, w, white)
	stroke(img, pt{385, 560}, pt{560, 330}, w, white)
	stroke(img, pt{560, 330}, pt{560, 704}, w, white)
	// Down-arrow — shaft plus two diagonal barbs, x ~650..850.
	stroke(img, pt{750, 330}, pt{750, 640}, w, white)
	stroke(img, pt{750, 700}, pt{630, 540}, w, white)
	stroke(img, pt{750, 700}, pt{870, 540}, w, white)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
