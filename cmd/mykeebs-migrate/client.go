package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// listPageLimit is the max the API accepts (api/openapi.yaml: Limit max 100).
const listPageLimit = 100

// apiClient is a thin wrapper over net/http for the mykeebs-api REST API: it attaches
// the bearer token, handles JSON encode/decode, and reports non-2xx responses
// with their body.
type apiClient struct {
	baseURL string
	token   string
	subject string
	http    *http.Client
	retry   throttleRetry
}

// newAPIClient builds a client for baseURL authenticating as token's subject.
func newAPIClient(baseURL, token string) (*apiClient, error) {
	sub, err := tokenSubject(token)
	if err != nil {
		return nil, err
	}
	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		subject: sub,
		http:    &http.Client{Timeout: 60 * time.Second},
		retry:   defaultThrottleRetry,
	}, nil
}

// apiError is returned for any non-2xx API response. It carries the status and
// body so callers can branch on, e.g., 404 or 409.
type apiError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// asAPIError reports whether err (or something it wraps) is an *apiError,
// assigning it to *target when so.
func asAPIError(err error, target **apiError) bool {
	return errors.As(err, target)
}

// doJSON performs an API request. reqBody, if non-nil, is JSON-encoded;
// respBody, if non-nil, is JSON-decoded from a 2xx response. path is relative
// to baseURL and must begin with "/".
func (c *apiClient) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encoding %s %s body: %w", method, path, err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("building %s %s: %w", method, path, err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.send(req)
	if err != nil {
		return fmt.Errorf("calling %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s %s response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	if respBody != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("decoding %s %s response: %w", method, path, err)
		}
	}
	return nil
}

// getRaw fetches path and returns the raw 2xx response body, for endpoints
// whose response is stored verbatim (entity GETs, GET /v1/lookups).
func (c *apiClient) getRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("building GET %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.send(req)
	if err != nil {
		return nil, fmt.Errorf("calling GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading GET %s response: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &apiError{Method: http.MethodGet, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	return raw, nil
}

// userPath builds "/v1/users/{subject}/{suffix}" for a collection route.
func (c *apiClient) userPath(suffix string) string {
	return "/v1/users/" + c.subject + "/" + strings.TrimLeft(suffix, "/")
}

// listPage is the shape every list endpoint returns. We only need the raw
// item objects and the cursor here.
type listPage struct {
	Items      []json.RawMessage `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

// listAll pages through a list endpoint at collectionPath (relative to
// baseURL), returning every item's raw JSON. Cursors are opaque and
// environment-specific, so they are only ever fed back to the same endpoint on
// the same environment.
func (c *apiClient) listAll(ctx context.Context, collectionPath string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	cursor := ""
	for {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(listPageLimit))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page listPage
		if err := c.doJSON(ctx, http.MethodGet, collectionPath+"?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			return all, nil
		}
		cursor = *page.NextCursor
	}
}
