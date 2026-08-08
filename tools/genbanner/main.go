// Command genbanner renders the Dr. Markdown, .MD 1600x420 README banner: a
// medical-scrubs teal full-bleed background with a subtle vertical gradient,
// the white "M↓" mark on the left (same stroke geometry as tools/genicon),
// and the full "Dr. Markdown, .MD" wordmark plus tagline on
// the right. The mark is pure stdlib polygon filling; the text is rendered
// with golang.org/x/image/font from a macOS system font.
//
// Usage: go run ./tools/genbanner -out docs/assets/banner.png
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

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const (
	width  = 1600
	height = 420
	margin = 80.0
)

var (
	tealTop = color.RGBA{17, 94, 89, 255}   // gradient start (darker)
	tealBot = color.RGBA{15, 118, 110, 255} // gradient end (lighter)
	white   = color.RGBA{255, 255, 255, 255}
	tint    = color.RGBA{153, 246, 228, 255} // light teal tint for tagline
)

// The mark's design-space bounding box, including stroke caps. Shared by
// glyph() and the layout in main() so the two cannot disagree about how much
// room the mark needs — they were separate literals once, and changing the
// mark updated only one of them.
const (
	markX0, markX1 = 174.0, 891.0
	markY0, markY1 = 294.0, 740.0
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

// bgAt returns the background gradient color at absolute row y.
func bgAt(y float64) color.RGBA {
	t := y / float64(height-1)
	return color.RGBA{
		R: uint8(float64(tealTop.R)*(1-t) + float64(tealBot.R)*t),
		G: uint8(float64(tealTop.G)*(1-t) + float64(tealBot.G)*t),
		B: uint8(float64(tealTop.B)*(1-t) + float64(tealBot.B)*t),
		A: 255,
	}
}

// background fills the canvas with a top-to-bottom teal gradient.
func background(img *image.RGBA) {
	for y := 0; y < height; y++ {
		draw.Draw(img, image.Rect(0, y, width, y+1), &image.Uniform{bgAt(float64(y))}, image.Point{}, draw.Src)
	}
}

// glyph draws the "M↓" mark with its bounding box (including stroke caps) at
// top-left (ox, oy), scaled to height h. It is the conventional markdown
// mark: a bold M followed by a down arrow sharing the M's stroke width, top
// and baseline, so the pair reads as one word. Returns the total width
// occupied.
//
// Keep both glyphs on one stroke width: the mark this replaced mixed a
// heavy M with finer curves and an inner disc, and lost all of it at small
// sizes.
func glyph(img *image.RGBA, ox, oy, h float64, c color.RGBA) float64 {
	// Local coordinate space: the M spans x 210..560, y 330..704 with stroke
	// width 72; including caps the mark's bounding box is x 174..891,
	// y 294..740.
	s := h / (markY1 - markY0)
	w := 72.0 * s // stroke width, shared by both glyphs
	at := func(x, y float64) pt { return pt{ox + (x-markX0)*s, oy + (y-markY0)*s} }
	// "M" — four strokes.
	stroke(img, at(210, 704), at(210, 330), w, c)
	stroke(img, at(210, 330), at(385, 560), w, c)
	stroke(img, at(385, 560), at(560, 330), w, c)
	stroke(img, at(560, 330), at(560, 704), w, c)
	// "↓" — stem and chevron on the M's top and baseline.
	const ax = 760.0
	stroke(img, at(ax, 330), at(ax, 610), w, c)
	stroke(img, at(ax-95, 522), at(ax, 704), w, c)
	stroke(img, at(ax+95, 522), at(ax, 704), w, c)
	return (markX1 - markX0) * s
}

// loadFonts tries each candidate path in order and returns (bold, regular)
// sfnt fonts plus a description of what was loaded.
func loadFonts() (bold, regular *sfnt.Font, desc string, err error) {
	type candidate struct {
		path string
		ttc  bool
	}
	candidates := []candidate{
		{"/System/Library/Fonts/Helvetica.ttc", true},
		{"/System/Library/Fonts/Supplemental/Arial Bold.ttf", false},
		{"/System/Library/Fonts/Supplemental/Arial.ttf", false},
	}
	for _, c := range candidates {
		data, rerr := os.ReadFile(c.path)
		if rerr != nil {
			continue
		}
		if c.ttc {
			coll, perr := sfnt.ParseCollection(data)
			if perr != nil {
				continue
			}
			var b, r *sfnt.Font
			for j := 0; j < coll.NumFonts(); j++ {
				f, ferr := coll.Font(j)
				if ferr != nil {
					continue
				}
				sub, _ := f.Name(nil, sfnt.NameIDSubfamily)
				switch {
				case b == nil && (sub == "Bold"):
					b = f
				case r == nil && (sub == "Regular"):
					r = f
				}
			}
			if r == nil {
				r, _ = coll.Font(0)
			}
			if b == nil {
				b = r
			}
			if b != nil && r != nil {
				return b, r, fmt.Sprintf("%s (collection, %d fonts)", c.path, coll.NumFonts()), nil
			}
			continue
		}
		f, perr := sfnt.Parse(data)
		if perr != nil {
			continue
		}
		// A single-face file serves both roles; prefer it as bold when the
		// path says Bold, otherwise keep it for regular and reuse for bold.
		return f, f, c.path, nil
	}
	return nil, nil, "", fmt.Errorf("no usable system font found")
}

func face(f *sfnt.Font, size float64) font.Face {
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatalf("creating face at %.1fpx: %v", size, err)
	}
	return fc
}

func f26(v fixed.Int26_6) float64 { return float64(v) / 64.0 }

func main() {
	out := flag.String("out", "docs/assets/banner.png", "output PNG path")
	flag.Parse()

	boldFont, regularFont, fontDesc, err := loadFonts()
	if err != nil {
		log.Fatalf("BLOCKED: %v", err)
	}
	log.Printf("font: %s", fontDesc)

	const wordmark = "Dr. Markdown, .MD"
	const tagline = "A native WYSIWYG markdown editor"

	// Target sizes, scaled down uniformly if the row would overflow.
	glyphH, wmSize, tagSize, gap := 260.0, 110.0, 48.0, 64.0
	glyphW := (markX1 - markX0) * (glyphH / (markY1 - markY0))

	measure := func(f *sfnt.Font, size float64, s string) float64 {
		fc := face(f, size)
		defer fc.Close()
		d := font.Drawer{Face: fc}
		return f26(d.MeasureString(s))
	}
	wmW := measure(boldFont, wmSize, wordmark)

	avail := width - 2*margin
	if need := glyphW + gap + wmW; need > avail {
		scale := avail / need
		glyphH *= scale
		wmSize *= scale
		gap *= scale
		glyphW *= scale
		wmW *= scale
	}
	textX := margin + glyphW + gap
	textColW := width - margin - textX
	tagW := measure(regularFont, tagSize, tagline)
	if tagW > textColW {
		tagSize *= textColW / tagW
		tagW = textColW
	}
	log.Printf("layout: glyph %.0fpx tall (%.0f wide), wordmark %.0fpx (%.0f wide), tagline %.0fpx (%.0f wide)",
		glyphH, glyphW, wmSize, wmW, tagSize, tagW)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	background(img)

	// Glyph, vertically centered.
	glyph(img, margin, (height-glyphH)/2, glyphH, white)

	// Text block, vertically centered as a unit.
	wmFace := face(boldFont, wmSize)
	defer wmFace.Close()
	tagFace := face(regularFont, tagSize)
	defer tagFace.Close()
	wmM, tagM := wmFace.Metrics(), tagFace.Metrics()
	lineGap := 0.35 * tagSize
	blockH := f26(wmM.Ascent+wmM.Descent) + lineGap + f26(tagM.Ascent+tagM.Descent)
	top := (height - blockH) / 2
	wmBaseline := top + f26(wmM.Ascent)
	tagBaseline := wmBaseline + f26(wmM.Descent) + lineGap + f26(tagM.Ascent)

	d := font.Drawer{Dst: img, Src: &image.Uniform{white}, Face: wmFace,
		Dot: fixed.P(int(textX), int(wmBaseline))}
	d.DrawString(wordmark)
	d.Src = &image.Uniform{tint}
	d.Face = tagFace
	d.Dot = fixed.P(int(textX), int(tagBaseline))
	d.DrawString(tagline)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
