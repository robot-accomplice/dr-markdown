// Command genicon renders the Dr. Markdown, .MD macOS icon set.
//
// ONE mark, at every size. The illustration this replaced — a stethoscope
// looped through a ring around the mark — was used from 64px up with a drawn
// mark below it, and the two did not read as the same product: the Dock showed
// a stethoscope and the Finder list showed a bare M.
//
// It was also unreadable, which is what actually retired it. The tubing crossed
// the letterforms, and this file's own note called two thin strokes
// intersecting the worst case for downsampling — then shipped exactly that.
// Its palette compounded it: mid-blue strokes on a pale blue tile have almost
// no contrast, so at Dock size it read as a smudge.
//
// What is here now is the mark the format itself uses — M with a down arrow —
// in white on a deep blue tile. Nothing crosses anything, the contrast is
// carried by the tile rather than by fine detail, and the same artwork is
// scaled to every slot, so resizing the Dock changes the size and nothing else.
//
// The cost is stated rather than hidden: the stethoscope carried the "Dr." of
// the name, and this mark does not. Legibility at 16px won that trade.
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

// White on a deep blue tile. Contrast is the whole point: the palette this
// replaced put mid-blue strokes on a pale blue tile, which is legible on a
// 1024px canvas and a smudge at 16px. The blue is the application's own accent
// darkened until white sits on it cleanly.
var (
	tile = color.RGBA{0x1a, 0x4a, 0x78, 255} // deep accent blue
	ink  = color.RGBA{0xff, 0xff, 0xff, 255} // white
)

// macOS icon grid: the squircle occupies 824 of a 1024 canvas, leaving the
// margin the OS expects around every app icon. Expressed as ratios so every
// size derives from the same geometry.
const (
	// The canvas every size is drawn on before being resampled down.
	designCanvas = 1024

	squircleRatio = 824.0 / 1024.0
	radiusRatio   = 185.4 / 824.0
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

// drawn renders the M-arrow mark on the tile, at every size.
//
// The geometry is chosen against the 16px slot rather than the 1024px one,
// because 16px is where a mark either survives or turns to mush and every
// larger size is forgiving. Two things were measured there:
//
//   - The M's notch must reach the BASELINE. With the vertex partway down, the
//     two inner diagonals merge at 16px and the letter reads as an H.
//   - The arrowhead must be a filled triangle. Drawn as two strokes meeting the
//     stem, it merges with the stem at 16px and reads as a plus sign.
//
// The stroke is thinner and the mark larger than a 1024px-first design would
// choose: weight that looks right on the hero is what closes the notch small.
func drawn(size int) *image.RGBA {
	const design = 1024.0
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{tile}, image.Point{}, draw.Src)

	const (
		markScale   = 0.78
		strokeRatio = 0.088
		mLeft       = 225.0 // the M's left stem
		mRight      = 600.0 // the M's right stem
		mVertex     = 740.0 // the notch, on the baseline
		arrowX      = 845.0
		headHalf    = 170.0 // half-width of the arrowhead
		headTop     = 580.0
		arrowTip    = 760.0
		glyphTop    = 300.0
		baseline    = 740.0
	)
	// Centred on the whole mark, both glyphs included, so it sits in the middle
	// of the tile rather than the M sitting in the middle and the arrow hanging.
	const centreX = (mLeft + arrowX + headHalf) / 2

	k := float64(size) / design * markScale
	c := float64(size) / 2
	P := func(x, y float64) pt { return pt{c + (x-centreX)*k, c + (y-520)*k} }
	w := design * strokeRatio * k

	// "M" — the notch runs to the baseline so it is still a notch at 16px.
	mMid := (mLeft + mRight) / 2
	stroke(img, P(mLeft, baseline), P(mLeft, glyphTop), w, ink)
	stroke(img, P(mLeft, glyphTop), P(mMid, mVertex), w, ink)
	stroke(img, P(mMid, mVertex), P(mRight, glyphTop), w, ink)
	stroke(img, P(mRight, glyphTop), P(mRight, baseline), w, ink)

	// "↓" — stem plus a filled head, sharing the M's top so the two glyphs sit
	// on one line rather than reading as two marks.
	stroke(img, P(arrowX, glyphTop), P(arrowX, headTop), w, ink)
	fillPoly(img, []pt{
		P(arrowX-headHalf, headTop),
		P(arrowX+headHalf, headTop),
		P(arrowX, arrowTip),
	}, ink)

	applySquircle(img)
	return img
}

// render draws the mark large and resamples it down.
//
// drawn() rasterises polygons with hard pixel edges — it has no antialiasing of
// its own — so asking it for a 16px icon directly produces exactly the mush
// this icon was replaced for: the M's notch and the arrowhead both close up.
// Drawing at 1024 and filtering down is what makes a 16px slot legible, and it
// is why the resampler is imported here.
//
// A size at or above the design canvas is drawn directly; there is nothing to
// gain from resampling 1:1.
func render(size int) *image.RGBA {
	if size >= designCanvas {
		return drawn(size)
	}
	large := drawn(designCanvas)
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(out, out.Bounds(), large, large.Bounds(), xdraw.Src, nil)
	return out
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

func main() {
	iconset := flag.String("iconset", "", "write a full .iconset directory here")
	out := flag.String("out", "", "write a single 1024px PNG here")
	flag.Parse()

	if *iconset == "" && *out == "" {
		log.Fatal("need -iconset or -out")
	}

	if *out != "" {
		writePNG(*out, render(1024))
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
		writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d.png", base, base)), render(base))
		writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d@2x.png", base, base)), render(base*2))
	}
}
