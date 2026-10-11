package moderation

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	"golang.org/x/image/webp"
)

// minDimension is Rekognition's smallest accepted width or height.
const minDimension = 80

// errUnprocessable marks an image that can never be moderated (it doesn't
// decode, is the wrong type, or is outside Rekognition's size limits), so
// it's rejected rather than retried.
var errUnprocessable = errors.New("unprocessable image")

// webpToJPEG re-encodes a WebP as JPEG, since Rekognition only takes JPEG
// and PNG. JPEG has no alpha, so transparent pixels are flattened onto
// white rather than the black they'd otherwise become.
func webpToJPEG(data []byte) ([]byte, error) {
	src, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: decoding webp: %w", errUnprocessable, err)
	}
	b := src.Bounds()
	if b.Dx() < minDimension || b.Dy() < minDimension {
		return nil, fmt.Errorf("%w: %dx%d is below %dpx", errUnprocessable, b.Dx(), b.Dy(), minDimension)
	}

	flat := image.NewRGBA(b)
	draw.Draw(flat, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, b, src, b.Min, draw.Over)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encoding jpeg: %w", err)
	}
	return out.Bytes(), nil
}
