package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ImagesSuite struct {
	suite.Suite
}

func TestImagesSuite(t *testing.T) {
	suite.Run(t, new(ImagesSuite))
}

func (s *ImagesSuite) TestExtForContentType() {
	cases := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/webp": ".webp",
		"image/gif":  ".gif",
	}
	for ct, want := range cases {
		got, err := extForContentType(ct)
		s.Require().NoError(err, ct)
		s.Equal(want, got, ct)
	}
}

func (s *ImagesSuite) TestExtForContentType_Unsupported() {
	_, err := extForContentType("image/tiff")
	s.Require().Error(err)
	s.ErrorContains(err, "unsupported")
}

func (s *ImagesSuite) TestDownloadImage_ReturnsBytesAndContentType() {
	body := []byte("\x89PNG fake bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	got, ct, err := downloadImage(context.Background(), srv.URL)
	s.Require().NoError(err)
	s.Equal(body, got)
	s.Equal("image/png", ct)
}

func (s *ImagesSuite) TestDownloadImage_Non2xxIsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer srv.Close()

	_, _, err := downloadImage(context.Background(), srv.URL)
	s.Require().Error(err)
	s.ErrorContains(err, "403")
}

func (s *ImagesSuite) TestUploadImage_SendsBytesAndHeaders() {
	var gotCT, gotIfNoneMatch string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	data := []byte("bytes to upload")
	err := uploadImage(context.Background(), srv.URL, "image/webp", data)
	s.Require().NoError(err)
	s.Equal("image/webp", gotCT)
	s.Equal("*", gotIfNoneMatch)
	s.Equal(data, gotBody)
}

func (s *ImagesSuite) TestUploadImage_Non2xxIsError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := uploadImage(context.Background(), srv.URL, "image/png", []byte("x"))
	s.Require().Error(err)
	s.ErrorContains(err, "500")
}

func (s *ImagesSuite) TestWriteAndVerify_RoundTrips() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "img.png")
	data := []byte("consistent bytes")
	s.Require().NoError(writeAndVerify(path, data, sha256Hex(data)))
}

func (s *ImagesSuite) TestWriteAndVerify_HashMismatchIsError() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "img.png")
	err := writeAndVerify(path, []byte("actual bytes"), sha256Hex([]byte("different bytes")))
	s.Require().Error(err)
	s.ErrorContains(err, "hash mismatch")
}
