package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gen2brain/webp"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

type ConvertSuite struct {
	suite.Suite
}

func TestConvertSuite(t *testing.T) {
	suite.Run(t, new(ConvertSuite))
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func (s *ConvertSuite) encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	s.Require().NoError(png.Encode(&buf, img))
	return buf.Bytes()
}

func (s *ConvertSuite) encodeWebP(img image.Image) []byte {
	var buf bytes.Buffer
	s.Require().NoError(webp.Encode(&buf, img, webp.Options{Quality: 90}))
	return buf.Bytes()
}

func (s *ConvertSuite) jpegWithOrientation(img image.Image, orientation uint16) []byte {
	var buf bytes.Buffer
	s.Require().NoError(jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}))
	encoded := buf.Bytes()

	var tiff bytes.Buffer
	tiff.WriteString("MM\x00\x2a")
	_ = binary.Write(&tiff, binary.BigEndian, uint32(8))
	_ = binary.Write(&tiff, binary.BigEndian, uint16(1))
	_ = binary.Write(&tiff, binary.BigEndian, []uint16{0x0112, 3})
	_ = binary.Write(&tiff, binary.BigEndian, uint32(1))
	_ = binary.Write(&tiff, binary.BigEndian, []uint16{orientation, 0})
	_ = binary.Write(&tiff, binary.BigEndian, uint32(0))
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)

	var out bytes.Buffer
	out.Write(encoded[:2])
	out.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&out, binary.BigEndian, uint16(len(payload)+2)) //nolint:gosec // a few dozen bytes.
	out.Write(payload)
	out.Write(encoded[2:])
	return out.Bytes()
}

func (s *ConvertSuite) decodeWebP(data []byte) image.Image {
	img, err := webp.Decode(bytes.NewReader(data))
	s.Require().NoError(err)
	return img
}

func (s *ConvertSuite) TestPNGOverMaxEdgeBecomesScaledWebP() {
	conv := &imageConverter{}

	out, err := conv.convert(s.encodePNG(solid(3000, 1000, color.White)))

	s.Require().NoError(err)
	s.Equal(image.Rect(0, 0, 2560, 853), s.decodeWebP(out).Bounds())
	s.Equal(1, conv.converted)
}

func (s *ConvertSuite) TestJPEGWithinMaxEdgeBecomesWebPAtSameSize() {
	conv := &imageConverter{}
	var buf bytes.Buffer
	s.Require().NoError(jpeg.Encode(&buf, solid(64, 32, color.White), nil))

	out, err := conv.convert(buf.Bytes())

	s.Require().NoError(err)
	s.Equal(image.Rect(0, 0, 64, 32), s.decodeWebP(out).Bounds())
}

func (s *ConvertSuite) TestJPEGOrientationIsApplied() {
	conv := &imageConverter{}
	img := solid(64, 32, color.RGBA{R: 255, A: 255})
	for y := range 32 {
		for x := 32; x < 64; x++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}

	out, err := conv.convert(s.jpegWithOrientation(img, 6))

	s.Require().NoError(err)
	got := s.decodeWebP(out)
	s.Equal(image.Rect(0, 0, 32, 64), got.Bounds())
	r, _, b, _ := got.At(16, 16).RGBA()
	s.Greater(r, b, "left half should be on top")
	r, _, b, _ = got.At(16, 48).RGBA()
	s.Greater(b, r, "right half should be at the bottom")
}

func (s *ConvertSuite) TestJPEGStoredAsPNGBecomesWebP() {
	conv := &imageConverter{}
	var buf bytes.Buffer
	s.Require().NoError(jpeg.Encode(&buf, solid(64, 32, color.White), nil))

	out, err := conv.convert(buf.Bytes())

	s.Require().NoError(err)
	s.Equal(image.Rect(0, 0, 64, 32), s.decodeWebP(out).Bounds())
}

func (s *ConvertSuite) TestWebPStoredAsPNGIsKept() {
	conv := &imageConverter{}
	data := s.encodeWebP(solid(16, 16, color.White))

	out, err := conv.convert(data)

	s.Require().NoError(err)
	s.Equal(data, out)
}

func (s *ConvertSuite) TestWebPWithinMaxEdgeIsUnchanged() {
	conv := &imageConverter{}
	data := s.encodeWebP(solid(2560, 10, color.White))

	out, err := conv.convert(data)

	s.Require().NoError(err)
	s.Equal(data, out)
	s.Zero(conv.converted)
}

func (s *ConvertSuite) TestWebPOverMaxEdgeIsScaled() {
	conv := &imageConverter{}

	out, err := conv.convert(s.encodeWebP(solid(100, 3000, color.White)))

	s.Require().NoError(err)
	s.Equal(image.Rect(0, 0, 85, 2560), s.decodeWebP(out).Bounds())
	s.Equal(1, conv.converted)
}

func (s *ConvertSuite) TestGIFIsRefused() {
	conv := &imageConverter{}
	var buf bytes.Buffer
	s.Require().NoError(gif.Encode(&buf, solid(16, 16, color.White), nil))

	_, err := conv.convert(buf.Bytes())

	s.ErrorContains(err, "image/gif")
}

func (s *ConvertSuite) TestAnimatedWebPIsRefused() {
	conv := &imageConverter{}
	var buf bytes.Buffer
	s.Require().NoError(webp.EncodeAll(&buf, &webp.WEBP{
		Image: []image.Image{solid(16, 16, color.White), solid(16, 16, color.Black)},
		Delay: []int{100, 100},
	}))

	_, err := conv.convert(buf.Bytes())

	s.ErrorContains(err, "animated")
}

func (s *ConvertSuite) TestWebPOverSizeCapIsRefused() {
	conv := &imageConverter{}
	noise := image.NewGray(image.Rect(0, 0, 1500, 1500))
	_, _ = rand.Read(noise.Pix)
	var buf bytes.Buffer
	s.Require().NoError(webp.Encode(&buf, noise, webp.Options{Lossless: true}))
	s.Require().Greater(buf.Len(), repository.MaxImageSizeBytes)

	_, err := conv.convert(buf.Bytes())

	s.ErrorContains(err, "cap")
}

func (s *ConvertSuite) TestOversizedCanvasIsRefusedBeforeDecoding() {
	conv := &imageConverter{}
	data := s.encodePNG(solid(1, 1, color.White))
	binary.BigEndian.PutUint32(data[16:], 20000)
	binary.BigEndian.PutUint32(data[20:], 20000)

	_, err := conv.convert(data)

	s.Require().Error(err)
	s.ErrorContains(err, "pixels")
}

func (s *ConvertSuite) TestOrientAndFitPlacesCorners() {
	src := solid(3, 2, color.Black)
	topLeft := color.RGBA{R: 255, A: 255}
	topRight := color.RGBA{G: 255, A: 255}
	src.Set(0, 0, topLeft)
	src.Set(2, 0, topRight)

	cases := []struct {
		orientation       int
		size              image.Point
		topLeft, topRight image.Point
	}{
		{1, image.Pt(3, 2), image.Pt(0, 0), image.Pt(2, 0)},
		{2, image.Pt(3, 2), image.Pt(2, 0), image.Pt(0, 0)},
		{3, image.Pt(3, 2), image.Pt(2, 1), image.Pt(0, 1)},
		{4, image.Pt(3, 2), image.Pt(0, 1), image.Pt(2, 1)},
		{5, image.Pt(2, 3), image.Pt(0, 0), image.Pt(0, 2)},
		{6, image.Pt(2, 3), image.Pt(1, 0), image.Pt(1, 2)},
		{7, image.Pt(2, 3), image.Pt(1, 2), image.Pt(1, 0)},
		{8, image.Pt(2, 3), image.Pt(0, 2), image.Pt(0, 0)},
	}
	for _, c := range cases {
		got := orientAndFit(src, c.orientation)
		s.Equal(c.size, got.Bounds().Size(), "orientation %d", c.orientation)
		s.Equal(topLeft, got.RGBAAt(c.topLeft.X, c.topLeft.Y), "orientation %d top-left", c.orientation)
		s.Equal(topRight, got.RGBAAt(c.topRight.X, c.topRight.Y), "orientation %d top-right", c.orientation)
	}
}

func (s *ConvertSuite) TestDownloadOneWritesConvertedImage() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(s.encodePNG(solid(8, 8, color.White)))
	}))
	defer srv.Close()
	dir := s.T().TempDir()

	entry, err := downloadOne(context.Background(), dir, "img-1", imageRef{imageID: "img-1", url: srv.URL}, &imageConverter{})

	s.Require().NoError(err)
	s.Equal("img-1.webp", entry.Filename)
	s.Equal("image/webp", entry.ContentType)
	written, err := os.ReadFile(filepath.Join(dir, "img-1.webp")) //nolint:gosec // test temp dir.
	s.Require().NoError(err)
	s.Equal(sha256Hex(written), entry.SHA256)
	s.Equal(len(written), entry.Bytes)
}
