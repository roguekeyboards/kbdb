package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// contentTypeExt maps the image content types the API's "image_content_type"
// lookup allows to a file extension. Kept explicit rather than using
// mime.ExtensionsByType so the on-disk names are stable and predictable.
var contentTypeExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// extForContentType returns the file extension for an image content type, or
// an error if it is not one the API accepts.
func extForContentType(ct string) (string, error) {
	if ext, ok := contentTypeExt[ct]; ok {
		return ext, nil
	}
	return "", fmt.Errorf("unsupported image content type %q", ct)
}

// sha256Hex is the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// httpClientForBlobs is used for signed image GETs and presigned S3 PUTs,
// which are plain HTTP, not API calls. Generous timeout for large originals.
var httpClientForBlobs = &http.Client{Timeout: 5 * time.Minute}

// downloadImage fetches a signed image URL and returns the bytes plus the
// response Content-Type.
func downloadImage(ctx context.Context, presignedURL string) (data []byte, contentType string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, presignedURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building image GET: %w", err)
	}
	resp, err := httpClientForBlobs.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading image body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("image GET returned HTTP %d: %s", resp.StatusCode, body)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// uploadImage PUTs data to a presigned S3 PUT URL with the given Content-Type,
// which must match the type the presign was locked to, and the
// If-None-Match: * the presign also requires.
func uploadImage(ctx context.Context, presignedURL, contentType string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, nil)
	if err != nil {
		return fmt.Errorf("building image PUT: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("If-None-Match", "*")

	resp, err := httpClientForBlobs.Do(req)
	if err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("image PUT returned HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// writeAndVerify writes data to path, then reads it back and re-hashes to
// confirm what landed on disk matches wantSHA. This is the loud-failure check
// the previous single-file dump lacked.
func writeAndVerify(path string, data []byte, wantSHA string) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	back, err := os.ReadFile(path) //nolint:gosec // path is inside the dump dir we control.
	if err != nil {
		return fmt.Errorf("re-reading %s: %w", path, err)
	}
	if got := sha256Hex(back); got != wantSHA {
		return fmt.Errorf("hash mismatch after writing %s: wrote %s, read back %s", path, wantSHA, got)
	}
	return nil
}
