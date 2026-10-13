package moderation

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	// maxPixels keeps the decode inside the moderation function's 512 MB:
	// a lossless WebP peaks at about 8 bytes a pixel while decoding.
	maxPixels = 40_000_000

	// maxSide keeps the JPEG well under Rekognition's 5 MiB Bytes limit,
	// and is still more detail than moderation needs.
	maxSide = 1600
)

// errUnprocessable marks an image that can never be moderated (it doesn't
// decode, is the wrong type, or Rekognition refuses it), so it's rejected
// rather than retried.
var errUnprocessable = errors.New("unprocessable image")

// webpToJPEG re-encodes a WebP as JPEG, since Rekognition only takes JPEG
// and PNG. The canvas size is checked before decoding, since a few KB of
// WebP can declare one too large to decode in memory.
func webpToJPEG(data []byte) ([]byte, error) {
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: reading webp header: %w", errUnprocessable, err)
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w: %dx%d is over %d pixels", errUnprocessable, cfg.Width, cfg.Height, maxPixels)
	}
	src, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: decoding webp: %w", errUnprocessable, err)
	}
	return toJPEG(src)
}

// toJPEG shrinks src to fit maxSide and encodes it. JPEG has no alpha, so
// transparent pixels are flattened onto white rather than the black they'd
// otherwise become.
func toJPEG(src image.Image) ([]byte, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longest := max(w, h); longest > maxSide {
		w = max(1, w*maxSide/longest)
		h = max(1, h*maxSide/longest)
	}

	flat := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(flat, flat.Bounds(), src, b, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encoding jpeg: %w", err)
	}
	return out.Bytes(), nil
}
