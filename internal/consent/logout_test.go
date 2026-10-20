package consent

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LogoutHandlerSuite struct {
	suite.Suite
}

func TestLogoutHandlerSuite(t *testing.T) {
	suite.Run(t, new(LogoutHandlerSuite))
}

func (s *LogoutHandlerSuite) TestGet_AllowedReturnTo_RendersIt() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to=http://localhost:5173/", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	s.Contains(rec.Body.String(), `"http://localhost:5173/"`)
}

func (s *LogoutHandlerSuite) TestGet_EmbedsBranding() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to=http://localhost:5173/", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	assertBranded(s.Assert(), rec.Body.String(), "mykeebs — Sign out")
}

func (s *LogoutHandlerSuite) TestGet_EscapesReturnTo() {
	// A return_to containing a double quote must not break out of the JS
	// string literal it's rendered into.
	returnTo := `http://localhost:5173/"; alert(1); //`
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to="+url.QueryEscape(returnTo), nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"http://localhost:5173/\"; alert(1); //"`)
}

func (s *LogoutHandlerSuite) TestGet_ReturnToScriptBreakout_DoesNotBreakOutOfScriptTag() {
	// A return_to containing a literal </script> must not close the
	// surrounding <script> block early - %q-style JS-string escaping alone
	// wouldn't stop this, since the HTML tokenizer scans for </script>
	// before any JS parsing happens. html/template's context-aware escaping
	// must rewrite it (e.g. to "<\/script>") to prevent breakout.
	returnTo := `http://localhost:5173/</script><script>alert(document.cookie)</script>`
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to="+url.QueryEscape(returnTo), nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.NotContains(rec.Body.String(), "</script><script>alert(document.cookie)</script>")
}

func (s *LogoutHandlerSuite) TestGet_MissingReturnTo_BadRequest() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/logout", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *LogoutHandlerSuite) TestGet_DisallowedReturnToOrigin_BadRequest() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to=https://evil.example.com/", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *LogoutHandlerSuite) TestGet_RelativeReturnTo_BadRequest() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet,
		"/logout?return_to=/some/path", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *LogoutHandlerSuite) TestNonGet_MethodNotAllowed() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost,
		"/logout?return_to=http://localhost:5173/", nil)
	rec := httptest.NewRecorder()

	LogoutHandler("public-token-test-abc123", "https://auth.example.com",
		[]string{"http://localhost:5173"}).ServeHTTP(rec, req)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal(http.MethodGet, rec.Header().Get("Allow"))
}
