package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// Client issues authenticated requests against support.BaseURL(). Zero
// value is usable directly (equivalent to &Client{}).
type Client struct{}

// NewClient returns a ready-to-use Client.
func NewClient() *Client {
	return &Client{}
}

// Do returns a response whose Body is pre-buffered in memory, not a live
// network read - Ginkgo cancels each node's context when that node's
// function returns (see the github.com/onsi/ginkgo/v2 module's
// internal/suite.go, runNode), and a request built with a BeforeEach's ctx
// used to fail decoding its still-live Body from a later It node once that
// BeforeEach had already returned.
func (c *Client) Do(ctx context.Context, method, path, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, support.BaseURL()+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(buf))

	return resp, nil
}

// DoPresigned sends a request to a presigned S3 URL. body and contentType
// are only meaningful for a PUT, which also sends the If-None-Match: *
// every presigned PUT requires. Package-level, not a Client method: unlike
// Do, it doesn't target support.BaseURL(), so it needs no client state.
func DoPresigned(ctx context.Context, method, presignedURL, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, presignedURL, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodPut {
		req.Header.Set("If-None-Match", "*")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(buf))

	return resp, nil
}
