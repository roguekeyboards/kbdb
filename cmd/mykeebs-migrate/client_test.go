package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ClientSuite struct {
	suite.Suite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

func (s *ClientSuite) testClient(baseURL string) *apiClient {
	c, err := newAPIClient(baseURL, unsignedJWT(map[string]any{"sub": "user_test"}))
	s.Require().NoError(err)
	return c
}

func (s *ClientSuite) TestNewAPIClient_ExtractsSubject() {
	c := s.testClient("https://api.example.test")
	s.Equal("user_test", c.subject)
	s.Equal("https://api.example.test", c.baseURL)
}

func (s *ClientSuite) TestUserPath() {
	c := s.testClient("https://api.example.test")
	s.Equal("/v1/users/user_test/keyboards", c.userPath("keyboards"))
	s.Equal("/v1/users/user_test/keyboards/k1", c.userPath("keyboards/k1"))
}

func (s *ClientSuite) TestListAll_FollowsCursorAndStopsOnNull() {
	// Three pages: cursor "" -> items 1,2 next "p2"; "p2" -> 3,4 next "p3";
	// "p3" -> 5 next null.
	p2, p3 := "p2", "p3"
	pages := map[string]listPage{
		"": {
			Items:      rawItems(`{"id":"1"}`, `{"id":"2"}`),
			NextCursor: &p2,
		},
		"p2": {
			Items:      rawItems(`{"id":"3"}`, `{"id":"4"}`),
			NextCursor: &p3,
		},
		"p3": {
			Items:      rawItems(`{"id":"5"}`),
			NextCursor: nil,
		},
	}

	var seenLimits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/v1/users/user_test/keyboards", r.URL.Path)
		seenLimits = append(seenLimits, r.URL.Query().Get("limit"))
		page, ok := pages[r.URL.Query().Get("cursor")]
		if !ok {
			http.Error(w, "unexpected cursor", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := s.testClient(srv.URL)
	items, err := c.listAll(context.Background(), c.userPath("keyboards"))
	s.Require().NoError(err)
	s.Len(items, 5)
	for _, l := range seenLimits {
		s.Equal(fmt.Sprint(listPageLimit), l)
	}
}

func (s *ClientSuite) TestDoJSON_Non2xxReturnsAPIError() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"conflict"}`))
	}))
	defer srv.Close()

	c := s.testClient(srv.URL)
	err := c.doJSON(context.Background(), http.MethodPost, "/v1/profile/user_test", map[string]string{"username": "x"}, nil)
	s.Require().Error(err)

	var apiErr *apiError
	s.Require().True(asAPIError(err, &apiErr))
	s.Equal(http.StatusConflict, apiErr.Status)
}

func (s *ClientSuite) throttlingServer(throttles int, contentType string, calls *int, bodies *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		body, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(body))
		if *calls <= throttles {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Too Many Requests"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"k1"}`))
	}))
}

func (s *ClientSuite) fastRetryClient(baseURL string) *apiClient {
	c := s.testClient(baseURL)
	c.retry = throttleRetry{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: time.Millisecond}
	return c
}

func (s *ClientSuite) TestDoJSON_RetriesThrottleWithTheSameBody() {
	var calls int
	var bodies []string
	srv := s.throttlingServer(2, "application/json", &calls, &bodies)
	defer srv.Close()

	var out struct{ ID string }
	err := s.fastRetryClient(srv.URL).doJSON(context.Background(), http.MethodPost, "/v1/users/user_test/keyboards", map[string]string{"name": "x"}, &out)
	s.Require().NoError(err)
	s.Equal("k1", out.ID)
	s.Equal(3, calls)
	s.Equal([]string{`{"name":"x"}`, `{"name":"x"}`, `{"name":"x"}`}, bodies)
}

func (s *ClientSuite) TestDoJSON_GivesUpAfterMaxAttempts() {
	var calls int
	var bodies []string
	srv := s.throttlingServer(3, "application/json", &calls, &bodies)
	defer srv.Close()

	err := s.fastRetryClient(srv.URL).doJSON(context.Background(), http.MethodGet, "/v1/users/user_test/keyboards", nil, nil)
	var apiErr *apiError
	s.Require().True(asAPIError(err, &apiErr))
	s.Equal(http.StatusTooManyRequests, apiErr.Status)
	s.Equal(3, calls)
}

func (s *ClientSuite) TestDoJSON_DoesNotRetryUploadLimit() {
	var calls int
	var bodies []string
	srv := s.throttlingServer(1, "application/problem+json", &calls, &bodies)
	defer srv.Close()

	err := s.fastRetryClient(srv.URL).doJSON(context.Background(), http.MethodPost, "/v1/users/user_test/keyboards/k1/images", nil, nil)
	var apiErr *apiError
	s.Require().True(asAPIError(err, &apiErr))
	s.Equal(http.StatusTooManyRequests, apiErr.Status)
	s.Equal(1, calls)
}

func (s *ClientSuite) TestGetRaw_RetriesThrottle() {
	var calls int
	var bodies []string
	srv := s.throttlingServer(1, "application/json", &calls, &bodies)
	defer srv.Close()

	raw, err := s.fastRetryClient(srv.URL).getRaw(context.Background(), "/v1/lookups")
	s.Require().NoError(err)
	s.JSONEq(`{"id":"k1"}`, string(raw))
	s.Equal(2, calls)
}

func rawItems(objs ...string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(objs))
	for _, o := range objs {
		out = append(out, json.RawMessage(o))
	}
	return out
}
