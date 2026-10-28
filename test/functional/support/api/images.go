package api

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/webp"
)

// MaxUploadBytes is the API's cap on an uploaded image.
const MaxUploadBytes = 1536 << 10

// RekognitionBytesLimit is Rekognition's limit on an image sent as Bytes.
const RekognitionBytesLimit = 5 << 20

// TestImageBytes is a real 100x100 WebP, the payload every spec PUTs to a
// presigned upload URL. It has to be one the moderation function decodes,
// or it's rejected, which hides it from the specs that read it back. The
// URL is signed for the size_bytes declared when it was minted, so specs
// declare len(TestImageBytes) (ImageUploadBody does) and PUT exactly these
// bytes.
var TestImageBytes = encodeWebP(grey100(128))

// ReplacementImageBytes is a real 100x100 WebP that differs from
// TestImageBytes, for the specs that replace one image with another.
var ReplacementImageBytes = encodeWebP(grey100(64))

// ImageUploadBody is an ImageUploadRequest JSON body for contentType, sized
// for TestImageBytes.
func ImageUploadBody(contentType string) string {
	return fmt.Sprintf(`{"content_type":%q,"size_bytes":%d}`, contentType, len(TestImageBytes))
}

// TestJPEG is a real 100x100 JPEG, for the specs that need an image that
// isn't a WebP.
var TestJPEG = func() []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, grey100(128), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

func grey100(level uint8) image.Image {
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	for i := range img.Pix {
		img.Pix[i] = level
	}
	return img
}

// BusyWebP returns a 2560x2560 WebP of random black and white pixels:
// about 1.3 MB, but over 6 MB as a full-size quality-90 JPEG, past what
// Rekognition takes as Bytes unless the moderation function shrinks it.
func BusyWebP() []byte {
	const side = 2560
	img := image.NewGray(image.Rect(0, 0, side, side))
	_, _ = rand.Read(img.Pix)
	for i, v := range img.Pix {
		img.Pix[i] = 255 * (v & 1)
	}
	return encodeWebP(img)
}

// OversizedWebP returns a single-colour 7000x7000 WebP: a few KB of file
// whose longest side is over the 2560px the moderation function allows.
func OversizedWebP() []byte {
	img := image.NewGray(image.Rect(0, 0, 7000, 7000))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	return encodeWebP(img)
}

func encodeWebP(img image.Image) []byte {
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// FullSizeJPEGLen is how many bytes the WebP data comes to as a full-size
// quality-90 JPEG, the moderation function's encoding before it shrinks.
func FullSizeJPEGLen(data []byte) int {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Len()
}

// WebPUploadBody is an ImageUploadRequest JSON body sized for data.
func WebPUploadBody(data []byte) string {
	return fmt.Sprintf(`{"content_type":"image/webp","size_bytes":%d}`, len(data))
}
