package help

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

func (s *HandlerSuite) get(method string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(s.T().Context(), method, "http://api.example.com/help/mcp", nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	MCPHandler().ServeHTTP(rec, req)
	return rec
}

func (s *HandlerSuite) TestGet_RendersPage() {
	rec := s.get(http.MethodGet, nil)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	s.Contains(body, "<title>mykeebs — Using mykeebs with an AI assistant</title>")
	s.Contains(body, "<h1>Using mykeebs with an AI assistant</h1>")
	s.Contains(body, "Setting it up")
}

func (s *HandlerSuite) TestGet_NamesServingHostsMCPURL() {
	body := s.get(http.MethodGet, nil).Body.String()

	s.Contains(body, "<code>https://api.example.com/mcp</code>")
	s.Contains(body, "claude mcp add --transport http mykeebs https://api.example.com/mcp")
}

func (s *HandlerSuite) TestGet_UsesForwardedProto() {
	body := s.get(http.MethodGet, http.Header{"X-Forwarded-Proto": {"http"}}).Body.String()

	s.Contains(body, "<code>http://api.example.com/mcp</code>")
}

func (s *HandlerSuite) TestGet_EmbedsBranding() {
	body := s.get(http.MethodGet, nil).Body.String()

	s.Contains(body, `<svg xmlns="http://www.w3.org/2000/svg"`)
	s.Contains(body, `href="data:image/svg`)
	s.Contains(body, "--accent: #e8a33d;")
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	rec := s.get(http.MethodPost, nil)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal(http.MethodGet, rec.Header().Get("Allow"))
}
