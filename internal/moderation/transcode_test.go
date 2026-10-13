package moderation

import (
	"bytes"
	"crypto/rand"
	"image"
	"image/jpeg"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TranscodeSuite struct {
	suite.Suite
}

func TestTranscodeSuite(t *testing.T) {
	suite.Run(t, new(TranscodeSuite))
}

func (s *TranscodeSuite) TestTransparentPixelsBecomeWhite() {
	// 100x100: the left half fully transparent, the right half opaque red.
	data, err := os.ReadFile("testdata/half-transparent.webp")
	s.Require().NoError(err)

	out, err := webpToJPEG(data)
	s.Require().NoError(err)

	img, err := jpeg.Decode(bytes.NewReader(out))
	s.Require().NoError(err)
	s.Equal(100, img.Bounds().Dx())

	r, g, b, _ := img.At(10, 50).RGBA()
	s.Greater(r>>8, uint32(240))
	s.Greater(g>>8, uint32(240))
	s.Greater(b>>8, uint32(240))

	r, g, b, _ = img.At(90, 50).RGBA()
	s.Greater(r>>8, uint32(180))
	s.Less(g>>8, uint32(40))
	s.Less(b>>8, uint32(40))
}

func (s *TranscodeSuite) TestOverPixelLimit_UnprocessableWithoutDecoding() {
	// A 35 KB lossless WebP that decodes to 7000x7000, over maxPixels.
	data, err := os.ReadFile("testdata/oversized.webp")
	s.Require().NoError(err)

	_, err = webpToJPEG(data)
	s.ErrorIs(err, errUnprocessable)
}

func (s *TranscodeSuite) TestLargeImage_ShrunkToMaxSide() {
	out, err := toJPEG(image.NewRGBA(image.Rect(0, 0, 3200, 2000)))
	s.Require().NoError(err)

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	s.Require().NoError(err)
	s.Equal(1600, cfg.Width)
	s.Equal(1000, cfg.Height)
}

func (s *TranscodeSuite) TestNoise_UnderRekognitionBytesLimit() {
	// Pure noise is the worst case for JPEG size; 2560 is the web UI's largest.
	img := image.NewRGBA(image.Rect(0, 0, 2560, 2560))
	_, _ = rand.Read(img.Pix)

	out, err := toJPEG(img)
	s.Require().NoError(err)
	s.LessOrEqual(len(out), 5<<20)
}

func (s *TranscodeSuite) TestNotWebP_Unprocessable() {
	_, err := webpToJPEG([]byte("not an image"))
	s.ErrorIs(err, errUnprocessable)
}
