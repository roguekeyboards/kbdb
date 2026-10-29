package about

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

func (s *HandlerSuite) get(method string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(s.T().Context(), method, "/about/ai", nil)
	rec := httptest.NewRecorder()
	AIHandler().ServeHTTP(rec, req)
	return rec
}

func (s *HandlerSuite) TestGet_RendersPage() {
	rec := s.get(http.MethodGet)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	body := rec.Body.String()
	s.Contains(body, "<title>mykeebs — How AI is used to build mykeebs</title>")
	s.Contains(body, "<h1>How AI is used to build mykeebs</h1>")
	s.Contains(body, "What the developer decided")
}

func (s *HandlerSuite) TestGet_EmbedsBranding() {
	body := s.get(http.MethodGet).Body.String()

	s.Contains(body, `<svg xmlns="http://www.w3.org/2000/svg"`)
	s.Contains(body, `href="data:image/svg`)
	s.Contains(body, "--accent: #e8a33d;")
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	rec := s.get(http.MethodPost)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal("GET, HEAD", rec.Header().Get("Allow"))
}

func (s *HandlerSuite) TestHead_OK() {
	rec := s.get(http.MethodHead)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
}
