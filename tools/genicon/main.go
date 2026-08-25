// Command genicon renders the Dr. Markdown, .MD macOS icon set.
//
// It writes a full .iconset — every size iconutil expects — rather than one
// 1024px source for the build to downscale, because the icon is DIFFERENT
// artwork at different sizes and that is deliberate.
//
// Why per-size artwork. The illustration (build/icon-artwork.png: a stethoscope
// looped through a ring around the M-arrow mark) is strong at 128px and above
// and unreadable below it. Measured, by rendering the candidate at the sizes
// that actually ship: at 32px the hairline tubing and the ring dissolve into a
// blue smudge with no readable content, and the tubing CROSSES the M, which is
// the worst case for downsampling — two thin strokes intersecting. The mark
// this project shipped before failed the same way, and the note it left behind
// said any future mark has to survive the Dock, not the 1024px source.
//
// So: the illustration is used from 64px up, and 16/32px get the bold M-arrow
// drawn here in the same palette. iconutil supports exactly this, and the
// alternative — one source scaled ten ways — forces the detail to survive 16px,
// which no amount of care in the artwork can achieve.
//
// The tile colour is constant across every size, so resizing the Dock changes
// the detail without the icon appearing to change colour.
//
// Usage:
//
//	go run ./tools/genicon -iconset build/icon.iconset
//	go run ./tools/genicon -out build/appicon.png          # 1024px hero only
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

// Sampled from build/icon-artwork.png rather than chosen: the tile is the
// artwork's own background, so the drawn sizes and the illustrated sizes agree
// without anyone matching them by eye. See docs/architext rules on branding.
var (
	tile = color.RGBA{0xaf, 0xdf, 0xf7, 255} // #afdff7 artwork background
	ink  = color.RGBA{0x1e, 0x53, 0x71, 255} // #1e5371 artwork stroke navy
)

// macOS icon grid: the squircle occupies 824 of a 1024 canvas, leaving the
// margin the OS expects around every app icon. Expressed as ratios so every
// size derives from the same geometry.
const (
	squircleRatio = 824.0 / 1024.0
	radiusRatio   = 185.4 / 824.0
	// Below this pixel size the illustration is not legible and the drawn
	// mark is used instead. 32px was measured as already unreadable.
	illustrationFloor = 64
)

type pt struct{ x, y float64 }

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

func thickLine(a, b pt, w float64) []pt {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l*w/2, dx/l*w/2
	return []pt{
		{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny}, {a.x - nx, a.y - ny},
	}
}

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

// squircleAlpha reports coverage at (x,y) for a rounded rect inset in a canvas
// of the given size. Returns 0..1 so edges can be antialiased; a hard mask at
// 16px produces visibly ragged corners.
func squircleAlpha(x, y int, size int) float64 {
	side := float64(size) * squircleRatio
	inset := (float64(size) - side) / 2
	r := side * radiusRatio
	px, py := float64(x)+0.5, float64(y)+0.5
	minX, minY := inset, inset
	maxX, maxY := inset+side, inset+side

	// Distance outside the rounded rectangle, negative inside.
	dx := math.Max(math.Max(minX+r-px, px-(maxX-r)), 0)
	dy := math.Max(math.Max(minY+r-py, py-(maxY-r)), 0)
	var d float64
	switch {
	case dx > 0 && dy > 0:
		d = math.Hypot(dx, dy) - r
	case px < minX || px > maxX:
		d = math.Max(minX-px, px-maxX)
	case py < minY || py > maxY:
		d = math.Max(minY-py, py-maxY)
	default:
		d = -1
	}
	// One pixel of feathering across the boundary.
	return math.Min(math.Max(0.5-d, 0), 1)
}

// applySquircle multiplies the image's alpha by the squircle mask.
func applySquircle(img *image.RGBA) {
	size := img.Bounds().Dx()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := squircleAlpha(x, y, size)
			c := img.RGBAAt(x, y)
			c.R = uint8(float64(c.R) * a)
			c.G = uint8(float64(c.G) * a)
			c.B = uint8(float64(c.B) * a)
			c.A = uint8(float64(c.A) * a)
			img.SetRGBA(x, y, c)
		}
	}
}

// illustrated scales the artwork to fill the squircle and masks it to shape.
func illustrated(art image.Image, size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// The artwork's own background is the tile colour, so it is scaled to the
	// FULL canvas and then masked: scaling it to the squircle box instead would
	// leave transparent corners with no colour to bleed into them.
	xdraw.CatmullRom.Scale(img, img.Bounds(), art, art.Bounds(), xdraw.Src, nil)
	applySquircle(img)
	return img
}

// drawn renders the bold M-arrow on the tile, for sizes the illustration
// cannot survive. Geometry is the mark this project already shipped, kept so
// the small icon is a simplification of the brand rather than a new mark.
func drawn(size int) *image.RGBA {
	const design = 1024.0
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{tile}, image.Point{}, draw.Src)

	// The mark is drawn at 88% about the canvas centre. At full size its box
	// spans 70% of the canvas against a squircle of 80%, which leaves under a
	// pixel of margin at 16px and reads as cramped against the tile edge.
	const markScale = 0.88
	k := float64(size) / design * markScale
	c := float64(size) / 2
	P := func(x, y float64) pt { return pt{c + (x-45-design/2)*k, c + (y-8-design/2)*k} }
	w := 84.0 * k

	// "M" — four bold strokes.
	stroke(img, P(240, 740), P(240, 300), w, ink)
	stroke(img, P(240, 300), P(380, 500), w, ink)
	stroke(img, P(380, 500), P(520, 300), w, ink)
	stroke(img, P(520, 300), P(520, 740), w, ink)
	// "↓" — stem and chevron at the M's weight, sharing its top and baseline
	// so the two glyphs sit on one line rather than reading as two marks.
	const ax = 760.0
	stroke(img, P(ax, 300), P(ax, 660), w, ink)
	stroke(img, P(ax-110, 566), P(ax, 740), w, ink)
	stroke(img, P(ax+110, 566), P(ax, 740), w, ink)

	applySquircle(img)
	return img
}

func render(art image.Image, size int) *image.RGBA {
	if size >= illustrationFloor {
		return illustrated(art, size)
	}
	return drawn(size)
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

func loadArtwork(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("artwork: %v", err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		log.Fatalf("artwork: %v", err)
	}
	if im.Bounds().Dx() != im.Bounds().Dy() {
		log.Fatalf("artwork must be square, got %dx%d", im.Bounds().Dx(), im.Bounds().Dy())
	}
	return im
}

func main() {
	artwork := flag.String("artwork", "build/icon-artwork.png", "source illustration (square)")
	iconset := flag.String("iconset", "", "write a full .iconset directory here")
	out := flag.String("out", "", "write a single 1024px PNG here")
	flag.Parse()

	if *iconset == "" && *out == "" {
		log.Fatal("need -iconset or -out")
	}
	art := loadArtwork(*artwork)

	if *out != "" {
		writePNG(*out, render(art, 1024))
	}
	if *iconset == "" {
		return
	}

	if err := os.MkdirAll(*iconset, 0o755); err != nil {
		log.Fatal(err)
	}
	// iconutil's expected slots. The @2x entry of one slot is the same pixel
	// count as the 1x entry of the next, and both are written from the same
	// render, so a 32px @2x and a 32px 1x are identical bytes by construction.
	for _, base := range []int{16, 32, 128, 256, 512} {
		writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d.png", base, base)), render(art, base))
		writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d@2x.png", base, base)), render(art, base*2))
	}
}
