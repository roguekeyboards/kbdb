package legal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type HandlerSuite struct {
	suite.Suite
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

func (s *HandlerSuite) get(h http.Handler, method string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(s.T().Context(), method, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (s *HandlerSuite) TestPrivacy_RendersPage() {
	rec := s.get(PrivacyHandler(), http.MethodGet)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	s.Contains(body, "<title>mykeebs — Privacy Policy</title>")
	s.Contains(body, "<h1>Privacy Policy</h1>")
	s.Contains(body, "What we collect")
}

func (s *HandlerSuite) TestTerms_RendersPage() {
	rec := s.get(TermsHandler(), http.MethodGet)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	s.Contains(body, "<title>mykeebs — Terms of Service</title>")
	s.Contains(body, "<h1>Terms of Service</h1>")
	s.Contains(body, "Who can use it")
}

func (s *HandlerSuite) TestGet_EmbedsBranding() {
	body := s.get(PrivacyHandler(), http.MethodGet).Body.String()

	s.Contains(body, `<svg xmlns="http://www.w3.org/2000/svg"`)
	s.Contains(body, `href="data:image/svg`)
	s.Contains(body, "--accent: #e8a33d;")
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	rec := s.get(TermsHandler(), http.MethodPost)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal("GET, HEAD", rec.Header().Get("Allow"))
}

func (s *HandlerSuite) TestHead_OK() {
	rec := s.get(PrivacyHandler(), http.MethodHead)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
}
