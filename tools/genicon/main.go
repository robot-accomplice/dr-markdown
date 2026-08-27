// Command genicon renders the Dr Markdown macOS icon set.
//
// ONE artwork, scaled to every slot. This file previously used a stethoscope
// illustration from 64px up and a mark drawn in Go below it, on the theory that
// the illustration could not survive small sizes. The theory was right about
// that illustration and wrong as a design: the Dock showed a stethoscope and
// the Finder list showed a bare M, so the two did not read as the same product.
//
// The artwork here is the mark the format itself uses — M with a down arrow, in
// white on a blue gradient — and it was checked at the size that decides the
// question before being adopted. At 16px the M's notch and the arrowhead both
// still resolve, which is exactly what the stethoscope failed to do: its tubing
// crossed the letterforms, and two thin strokes intersecting is the worst case
// for downsampling.
//
// So there is no per-size branch any more, and nothing here draws. Resampling a
// legible mark is all that is needed, and the artwork is the single source of
// what the icon looks like.
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
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

// macOS icon grid: the squircle occupies 824 of a 1024 canvas, leaving the
// margin the OS expects around every app icon. Expressed as ratios so every
// size derives from the same geometry.
const (
	squircleRatio = 824.0 / 1024.0
	radiusRatio   = 185.4 / 824.0
)

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

// render scales the artwork to the requested size and masks it to the squircle.
//
// The artwork is scaled to the FULL canvas and then masked, not scaled to the
// squircle's box: the artwork carries its own tile to the edge, so fitting it
// inside the box would leave transparent corners with no colour to bleed into
// them.
func render(art image.Image, size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(img, img.Bounds(), art, art.Bounds(), xdraw.Src, nil)
	applySquircle(img)
	return img
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
