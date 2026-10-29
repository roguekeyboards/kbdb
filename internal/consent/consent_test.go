package consent

import (
	"crypto/sha512"
	"encoding/base64"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (s *HandlerSuite) TestGet_LinksPrivacyAndTerms() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `href="https://mykeebs.com/privacy"`)
	s.Contains(rec.Body.String(), `href="https://mykeebs.com/terms"`)
}

func (s *HandlerSuite) TestGet_LoadsFontsFromMykeebs() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	body := rec.Body.String()
	s.Contains(body, "src: url('https://mykeebs.com/fonts/v1/jetbrains-mono-latin-700.woff2') format('woff2')")
	s.Contains(body, "src: url('https://mykeebs.com/fonts/v1/inter-latin-400.woff2') format('woff2')")
	s.Contains(body, "font-display: swap")
	s.NotContains(body, "fonts.googleapis.com")
}

func (s *HandlerSuite) TestGet_LoadsIDPScriptWithIntegrity() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	sum := sha512.Sum384(idpScript)
	s.Contains(html.UnescapeString(rec.Body.String()), `<script src="`+IDPScriptPath+`" integrity="sha384-`+base64.StdEncoding.EncodeToString(sum[:])+`"></script>`)
	s.NotContains(rec.Body.String(), "esm.sh")
}

func (s *HandlerSuite) TestIDPScript_ServesSDK() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodGet, IDPScriptPath, nil)
	rec := httptest.NewRecorder()

	IDPScriptHandler().ServeHTTP(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("text/javascript; charset=utf-8", rec.Header().Get("Content-Type"))
	s.Equal("public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	s.True(strings.HasPrefix(rec.Body.String(), "var stytch=function(e){"))
}

func (s *HandlerSuite) TestNonGet_MethodNotAllowed() {
	req := httptest.NewRequestWithContext(s.T().Context(), http.MethodPost, "/authorize", nil)
	rec := httptest.NewRecorder()

	Handler("public-token-test-abc123", "https://auth.example.com").ServeHTTP(rec, req)

	s.Equal(http.StatusMethodNotAllowed, rec.Code)
	s.Equal(http.MethodGet, rec.Header().Get("Allow"))
}
