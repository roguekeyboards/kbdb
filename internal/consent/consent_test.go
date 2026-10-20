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

func (s *HandlerSuite) TestGet_RendersPublicToken() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	s.Contains(rec.Body.String(), `"public-token-test-abc123"`)
}

func (s *HandlerSuite) TestGet_EscapesPublicToken() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	// A token containing a double quote must not break out of the JS
	// string literal it's rendered into.
	Handler(`abc"; alert(1); //`, "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"abc\"; alert(1); //"`)
}

func (s *HandlerSuite) TestGet_IncludesSwitchAccountControl() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	// The "not you?" control that lets a wrong cached session be swapped
	// before the consent card forces a decision.
	s.Contains(rec.Body.String(), `id="switch-account"`)
	s.Contains(rec.Body.String(), "Use a different account")
}

func (s *HandlerSuite) TestGet_EmbedsBranding() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	assertBranded(s.Assert(), rec.Body.String(), "mykeebs — Sign in")
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal(http.MethodGet, rec.Header().Get("Allow"))
}
