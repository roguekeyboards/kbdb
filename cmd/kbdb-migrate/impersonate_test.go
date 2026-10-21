package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ImpersonateSuite struct {
	suite.Suite

	base string
	got  chan sessionResult
}

type sessionResult struct {
	jwt string
	err error
}

func TestImpersonateSuite(t *testing.T) {
	suite.Run(t, new(ImpersonateSuite))
}

// SetupTest starts waitForSession on a free port; its outcome arrives on
// s.got.
func (s *ImpersonateSuite) SetupTest() {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	s.base = "http://" + ln.Addr().String()
	got := make(chan sessionResult, 1)
	s.got = got
	ctx, cancel := context.WithCancel(context.Background())
	s.T().Cleanup(cancel)
	go func() {
		jwt, err := waitForSession(ctx, ln, "https://auth.example.com", "public-token-test-1")
		got <- sessionResult{jwt, err}
	}()
}

// page fetches the impersonation page.
func (s *ImpersonateSuite) page() string {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.base+impersonationPath+"?token=t", nil)
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	return string(body)
}

// nonce reads the nonce the page was rendered with.
func (s *ImpersonateSuite) nonce() string {
	m := regexp.MustCompile(`nonce: "([^"]+)"`).FindStringSubmatch(s.page())
	s.Require().Len(m, 2)
	return m[1]
}

func (s *ImpersonateSuite) post(body string) int {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.base+sessionPath, strings.NewReader(body))
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func (s *ImpersonateSuite) TestPage_QuotesTheStytchSettings() {
	page := s.page()

	s.Contains(page, `createStytchClient("public-token-test-1", { customBaseUrl: "https://auth.example.com" })`)
}

func (s *ImpersonateSuite) TestSession_ReturnsThePostedJWT() {
	status := s.post(`{"nonce":"` + s.nonce() + `","session_jwt":"a.b.c"}`)

	s.Equal(http.StatusNoContent, status)
	res := <-s.got
	s.Require().NoError(res.err)
	s.Equal("a.b.c", res.jwt)
}

func (s *ImpersonateSuite) TestSession_WrongNonceIsRejected() {
	s.nonce()

	status := s.post(`{"nonce":"guess","session_jwt":"a.b.c"}`)

	s.Equal(http.StatusBadRequest, status)
	s.Empty(s.got)
}

func (s *ImpersonateSuite) TestSession_PageErrorIsReturned() {
	s.post(`{"nonce":"` + s.nonce() + `","error":"invalid token"}`)

	res := <-s.got
	s.Require().ErrorContains(res.err, "invalid token")
}
