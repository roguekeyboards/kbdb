package api

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// BuildsClient calls the /v1/users/{userId}/builds routes. Built on Client
// so specs don't hand-build requests/auth headers per route.
type BuildsClient struct {
	client *Client
}

// NewBuildsClient returns a ready-to-use BuildsClient.
func NewBuildsClient() *BuildsClient {
	return &BuildsClient{client: NewClient()}
}

// Create calls POST /v1/users/{ownerID}/builds.
func (c *BuildsClient) Create(ctx context.Context, ownerID, token, body string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodPost, "/v1/users/"+ownerID+"/builds", token, bytes.NewBufferString(body))
}

// Get calls GET /v1/users/{ownerID}/builds/{id}.
func (c *BuildsClient) Get(ctx context.Context, ownerID, id, token string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodGet, "/v1/users/"+ownerID+"/builds/"+id, token, nil)
}

// Update calls PUT /v1/users/{ownerID}/builds/{id}.
func (c *BuildsClient) Update(ctx context.Context, ownerID, id, token, body string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodPut, "/v1/users/"+ownerID+"/builds/"+id, token, bytes.NewBufferString(body))
}

// Delete calls DELETE /v1/users/{ownerID}/builds/{id}.
func (c *BuildsClient) Delete(ctx context.Context, ownerID, id, token string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodDelete, "/v1/users/"+ownerID+"/builds/"+id, token, nil)
}

// List sends limit as the query parameter when >= 0; a negative limit
// omits it, letting the server apply its default.
func (c *BuildsClient) List(ctx context.Context, ownerID, token string, limit int) (*http.Response, error) {
	return c.ListFiltered(ctx, ownerID, token, "", limit)
}

// ListFiltered is List plus an optional keywordID filter (keyboard_id query
// param); an empty keyboardID omits it.
func (c *BuildsClient) ListFiltered(ctx context.Context, ownerID, token, keyboardID string, limit int) (*http.Response, error) {
	return c.ListPage(ctx, ownerID, token, keyboardID, limit, "")
}

// ListPage is ListFiltered resuming from cursor; an empty cursor omits it.
func (c *BuildsClient) ListPage(ctx context.Context, ownerID, token, keyboardID string, limit int, cursor string) (*http.Response, error) {
	path := "/v1/users/" + ownerID + "/builds"

	query := url.Values{}
	if limit >= 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if keyboardID != "" {
		query.Set("keyboard_id", keyboardID)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	return c.client.Do(ctx, http.MethodGet, path, token, nil)
}

// AddImage calls POST /v1/users/{ownerID}/builds/{id}/images.
func (c *BuildsClient) AddImage(ctx context.Context, ownerID, id, token, body string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodPost, "/v1/users/"+ownerID+"/builds/"+id+"/images", token, bytes.NewBufferString(body))
}

// DeleteImage calls DELETE /v1/users/{ownerID}/builds/{id}/images/{imageID}.
func (c *BuildsClient) DeleteImage(ctx context.Context, ownerID, id, imageID, token string) (*http.Response, error) {
	return c.client.Do(ctx, http.MethodDelete, "/v1/users/"+ownerID+"/builds/"+id+"/images/"+imageID, token, nil)
}
