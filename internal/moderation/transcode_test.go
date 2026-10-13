package moderation

import (
	"bytes"
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

func (s *TranscodeSuite) TestNotWebP_Unprocessable() {
	_, err := webpToJPEG([]byte("not an image"))
	s.ErrorIs(err, errUnprocessable)
}
