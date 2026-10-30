package consent

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

func (s *HandlerSuite) TestGet_RendersConfig() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	s.Contains(rec.Body.String(), `{"publicToken":"public-token-test-abc123","issuerBaseUrl":"https://auth.example.com"}`)
}

func (s *HandlerSuite) TestGet_ServesTheBrandedPage() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Contains(rec.Body.String(), "<title>mykeebs — Sign in</title>")
	s.Contains(rec.Body.String(), `id="switch-account"`)
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal(http.MethodGet, rec.Header().Get("Allow"))
}
