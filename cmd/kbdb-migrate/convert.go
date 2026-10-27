package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"

	"github.com/bep/imagemeta"
	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

// maxEdge and webpQuality match mykeebs-web's src/lib/image-resize.ts, so a
// migrated image is what uploading it through the web app would have stored.
const (
	maxEdge     = 2560
	webpQuality = 82

	// maxDecodePixels refuses a canvas too large to decode in memory, which
	// a small file can declare.
	maxDecodePixels = 100_000_000
)

// imageConverter rewrites dumped images as WebP no larger than maxEdge on
// either side, counting each one it changes.
type imageConverter struct {
	enabled   bool
	converted int
}

// convert returns the bytes and content type to dump for an image. JPEG and
// PNG become WebP, upright and scaled to fit maxEdge, and a WebP over maxEdge
// is scaled and re-encoded. Any other image, an animated one, a WebP within
// maxEdge, or anything at all when conversion is disabled is returned as is.
// The format comes from the bytes, not ct: some images were stored with
// another format's content type, and a kept one is returned with its real one.
func (c *imageConverter) convert(data []byte, ct string) ([]byte, string, error) {
	if !c.enabled {
		return data, ct, nil
	}
	sniffed := http.DetectContentType(data)
	var format imagemeta.ImageFormat
	switch sniffed {
	case "image/jpeg":
		format = imagemeta.JPEG
	case "image/png":
		format = imagemeta.PNG
	case "image/webp":
		format = imagemeta.WebP
	default:
		return data, ct, nil
	}
	ct = sniffed

	meta, err := readImageMeta(data, format)
	if err != nil {
		return nil, "", err
	}
	if meta.frames > 1 {
		return data, ct, nil
	}
	if format == imagemeta.WebP && max(meta.width, meta.height) <= maxEdge {
		return data, ct, nil
	}
	if meta.width*meta.height > maxDecodePixels {
		return nil, "", fmt.Errorf("%dx%d is over %d pixels", meta.width, meta.height, maxDecodePixels)
	}

	var src image.Image
	switch format {
	case imagemeta.JPEG:
		src, err = jpeg.Decode(bytes.NewReader(data))
	case imagemeta.PNG:
		src, err = png.Decode(bytes.NewReader(data))
	default:
		src, err = webp.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, "", fmt.Errorf("decoding %s: %w", ct, err)
	}

	var out bytes.Buffer
	if err := webp.Encode(&out, orientAndFit(src, meta.orientation), webp.Options{Quality: webpQuality}); err != nil {
		return nil, "", fmt.Errorf("encoding webp: %w", err)
	}
	c.converted++
	return out.Bytes(), "image/webp", nil
}

type imageMeta struct {
	width, height int
	frames        int
	orientation   int
}

// readImageMeta reads an image's dimensions, frame count and EXIF
// orientation without decoding its pixels.
func readImageMeta(data []byte, format imagemeta.ImageFormat) (imageMeta, error) {
	meta := imageMeta{orientation: 1}
	res, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(data),
		ImageFormat: format,
		Sources:     imagemeta.EXIF | imagemeta.CONFIG,
		HandleTag: func(ti imagemeta.TagInfo) error {
			if v, ok := ti.Value.(uint16); ok && ti.Tag == "Orientation" && v >= 1 && v <= 8 {
				meta.orientation = int(v)
			}
			return nil
		},
	})
	if err != nil {
		return imageMeta{}, fmt.Errorf("reading %s metadata: %w", format, err)
	}
	meta.width, meta.height, meta.frames = res.ImageConfig.Width, res.ImageConfig.Height, res.ImageConfig.FrameCount
	return meta, nil
}

// orientAndFit draws src upright, as EXIF orientation o says to display it,
// scaled down to fit maxEdge, in a single resample.
func orientAndFit(src image.Image, o int) *image.RGBA {
	b := src.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	uw, uh := w, h
	if o >= 5 {
		uw, uh = h, w
	}
	scale := min(1, maxEdge/max(uw, uh))
	dw := max(1, int(math.Round(uw*scale)))
	dh := max(1, int(math.Round(uh*scale)))
	sx, sy := float64(dw)/uw, float64(dh)/uh

	m := orientation(o, w, h)
	mx, my := float64(b.Min.X), float64(b.Min.Y)
	s2d := f64.Aff3{
		sx * m[0], sx * m[1], sx * (m[2] - m[0]*mx - m[1]*my),
		sy * m[3], sy * m[4], sy * (m[5] - m[3]*mx - m[4]*my),
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Transform(dst, s2d, src, b, draw.Src, nil)
	return dst
}

// orientation maps a w×h image onto its upright form for EXIF orientation o.
func orientation(o int, w, h float64) f64.Aff3 {
	switch o {
	case 2: // mirrored horizontally
		return f64.Aff3{-1, 0, w, 0, 1, 0}
	case 3: // rotated 180°
		return f64.Aff3{-1, 0, w, 0, -1, h}
	case 4: // mirrored vertically
		return f64.Aff3{1, 0, 0, 0, -1, h}
	case 5: // mirrored along the top-left to bottom-right diagonal
		return f64.Aff3{0, 1, 0, 1, 0, 0}
	case 6: // needs a 90° clockwise turn
		return f64.Aff3{0, -1, h, 1, 0, 0}
	case 7: // mirrored along the top-right to bottom-left diagonal
		return f64.Aff3{0, -1, h, -1, 0, w}
	case 8: // needs a 90° anticlockwise turn
		return f64.Aff3{0, 1, 0, -1, 0, w}
	default:
		return f64.Aff3{1, 0, 0, 0, 1, 0}
	}
}
