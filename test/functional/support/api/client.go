package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// presignedURLHostRewrite maps the internal Docker hostname the Lambda
// container signs S3 presigned URLs against to the host-published port the
// Ginkgo suite (running outside the compose network) can actually reach.
// LocalStack has no server-side fix for this (a presigned URL's host is
// baked in at signing time, before LocalStack is ever involved) - this
// rewrite is paired with docker-compose.yml's S3_SKIP_SIGNATURE_VALIDATION,
// since the rewritten request's Host no longer matches what was signed.
// Local/CI-only: production presigned URLs always target real S3, which
// this string never matches, so DoPresigned is a plain passthrough there.
const presignedURLHostRewrite = "localstack:4566"

// DoPresigned rewrites a LocalStack-internal host to its host-published
// equivalent first - see presignedURLHostRewrite. body and contentType are
// only meaningful for a PUT, which also sends the If-None-Match: * every
// presigned PUT requires. Package-level, not a Client method: unlike Do,
// it doesn't target support.BaseURL(), so it needs no client state.
func DoPresigned(ctx context.Context, method, presignedURL, contentType string, body io.Reader) (*http.Response, error) {
	presignedURL = strings.Replace(presignedURL, presignedURLHostRewrite, "localhost:4566", 1)

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
