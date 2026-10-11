package api

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// TestImageBytes is the payload every spec PUTs to a presigned upload URL.
// The URL is signed for the size_bytes declared when it was minted, so
// specs declare len(TestImageBytes) (ImageUploadBody does) and PUT exactly
// these bytes.
var TestImageBytes = []byte("fake-image-bytes-for-testing")

// ImageUploadBody is an ImageUploadRequest JSON body for contentType, sized
// for TestImageBytes.
func ImageUploadBody(contentType string) string {
	return fmt.Sprintf(`{"content_type":%q,"size_bytes":%d}`, contentType, len(TestImageBytes))
}

// TestJPEG is a real 100x100 JPEG, for specs where the moderation function
// has to read the upload: Rekognition refuses TestImageBytes outright.
var TestJPEG = func() []byte {
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// JPEGUploadBody is an ImageUploadRequest JSON body sized for TestJPEG.
func JPEGUploadBody() string {
	return fmt.Sprintf(`{"content_type":"image/jpeg","size_bytes":%d}`, len(TestJPEG))
}
