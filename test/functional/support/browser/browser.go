// Package browser drives the consent page in headless Chromium, with the
// IdP's API answered by a stub: the stacks under test aren't connected to a
// real IdP project, and a real Discord sign-in can't be automated.
package browser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// PublicToken is the IdpConsentPublicToken scripts/func-setup.sh and ci.yml
// deploy the stack under test with.
const PublicToken = "public-token-test-local-kbdb" //nolint:gosec // a deploy parameter for test stacks, not a credential.

// Browser is one headless Chromium for a suite process.
type Browser struct {
	pw      *playwright.Playwright
	browser playwright.Browser
}

// Start launches Chromium. `go tool playwright install chromium` (run by
// scripts/func-test.sh) must have installed it.
func Start() (*Browser, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}
	b, err := pw.Chromium.Launch()
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("launching chromium: %w", err)
	}
	return &Browser{pw: pw, browser: b}, nil
}

// Stop closes Chromium.
func (b *Browser) Stop() {
	_ = b.browser.Close()
	_ = b.pw.Stop()
}

// Options set what the stub IdP and the network look like to a Session.
type Options struct {
	// SignedIn makes the browser hold an IdP session, as after a sign-in.
	SignedIn bool
	// BlockFonts makes mykeebs.com's fonts unreachable.
	BlockFonts bool
}

// IDPRequest is one call the page made to the IdP's API.
type IDPRequest struct {
	Path string
	URL  *url.URL
	Body map[string]any
}

// Session is a fresh browser profile with one page open on nothing yet.
type Session struct {
	Page playwright.Page

	context playwright.BrowserContext
	origin  string
	idpBase string

	mu         sync.Mutex
	signedIn   bool
	idp        []IDPRequest
	elsewhere  []string
	pageErrors []string
}

// NewSession opens a browser profile whose calls to the IdP's API at
// support.OidcIssuer(), the base URL the page gives the SDK, are answered by
// the stub. Requests anywhere else other than the stack and mykeebs.com's
// fonts, including the IdP's API on any other host, are refused and recorded.
func (b *Browser) NewSession(opts Options) (*Session, error) {
	base, err := url.Parse(support.BaseURL())
	if err != nil {
		return nil, err
	}
	ctx, err := b.browser.NewContext()
	if err != nil {
		return nil, err
	}
	s := &Session{
		context:  ctx,
		origin:   base.Scheme + "://" + base.Host,
		idpBase:  strings.TrimSuffix(support.OidcIssuer(), "/") + "/",
		signedIn: opts.SignedIn,
	}

	if opts.SignedIn {
		state, err := json.Marshal(map[string]any{"session": stubSession, "user": stubUser})
		if err != nil {
			return nil, err
		}
		script := fmt.Sprintf("localStorage.setItem(%q, %q)", "stytch_sdk_state_"+PublicToken, state)
		if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
			return nil, err
		}
		if err := ctx.AddCookies([]playwright.OptionalCookie{{
			Name: "stytch_session", Value: "session-token-test", URL: playwright.String(s.origin),
		}}); err != nil {
			return nil, err
		}
	}

	if err := ctx.Route("**/*", func(r playwright.Route) { s.route(r, opts) }); err != nil {
		return nil, err
	}

	page, err := ctx.NewPage()
	if err != nil {
		return nil, err
	}
	page.On("pageerror", func(e error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.pageErrors = append(s.pageErrors, e.Error())
	})
	s.Page = page
	return s, nil
}

// Close discards the browser profile.
func (s *Session) Close() {
	_ = s.context.Close()
}

// Open navigates to path on the stack under test, e.g. "/authorize?…".
func (s *Session) Open(path string) error {
	_, err := s.Page.Goto(support.BaseURL() + path)
	return err
}

// IDPRequests returns the page's calls to the IdP's API whose path ends in
// suffix, in order.
func (s *Session) IDPRequests(suffix string) []IDPRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []IDPRequest
	for _, r := range s.idp {
		if strings.HasSuffix(r.Path, suffix) {
			out = append(out, r)
		}
	}
	return out
}

// Elsewhere returns the URLs the page tried to load from any other origin.
func (s *Session) Elsewhere() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.elsewhere...)
}

// PageErrors returns the page's uncaught script errors.
func (s *Session) PageErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.pageErrors...)
}

func (s *Session) route(r playwright.Route, opts Options) {
	req := r.Request()
	u, err := url.Parse(req.URL())
	if err != nil {
		_ = r.Abort()
		return
	}

	switch {
	case u.Scheme+"://"+u.Host == s.origin:
		_ = r.Continue()
	case u.Host == "mykeebs.com" && strings.HasPrefix(u.Path, "/fonts/"):
		if opts.BlockFonts {
			_ = r.Abort()
			return
		}
		_ = r.Continue()
	case strings.HasPrefix(u.Scheme+"://"+u.Host+u.Path, s.idpBase):
		s.answer(r, u)
	default:
		s.mu.Lock()
		s.elsewhere = append(s.elsewhere, u.String())
		s.mu.Unlock()
		_ = r.Abort()
	}
}

func (s *Session) answer(r playwright.Route, u *url.URL) {
	var body map[string]any
	if data, err := r.Request().PostData(); err == nil && data != "" {
		_ = json.Unmarshal([]byte(data), &body)
	}

	s.mu.Lock()
	s.idp = append(s.idp, IDPRequest{Path: u.Path, URL: u, Body: body})
	signedIn := s.signedIn
	s.mu.Unlock()

	path := u.Path
	switch {
	case strings.Contains(path, "/v1/public/oauth/discord/start"):
		_ = r.Fulfill(playwright.RouteFulfillOptions{
			Status: playwright.Int(200), ContentType: playwright.String("text/html"), Body: "IdP OAuth start",
		})
	case strings.Contains(path, "/sdk/v1/projects/bootstrap/"):
		s.json(r, 200, map[string]any{"data": stubBootstrap})
	case strings.HasSuffix(path, "/sdk/v1/events"):
		_ = r.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(204), Headers: s.cors()})
	case strings.HasSuffix(path, "/sdk/v1/sessions/authenticate"):
		if !signedIn {
			s.json(r, 401, map[string]any{"error_type": "user_unauthenticated", "error_message": "no session"})
			return
		}
		s.json(r, 200, map[string]any{"data": stubAuthenticated})
	case strings.HasSuffix(path, "/sdk/v1/oauth/authenticate"):
		s.mu.Lock()
		s.signedIn = true
		s.mu.Unlock()
		s.json(r, 200, map[string]any{"data": stubAuthenticated})
	case strings.HasSuffix(path, "/sdk/v1/idp/oauth/authorize/start"):
		s.json(r, 200, map[string]any{"data": map[string]any{
			"consent_required": true,
			"client":           map[string]any{"client_id": body["client_id"], "client_name": "Test Client"},
			"scope_results":    []map[string]any{{"scope": "openid", "description": "Verify your identity", "is_grantable": true}},
		}})
	case strings.HasSuffix(path, "/sdk/v1/oauth/logout/start"):
		s.json(r, 200, map[string]any{"data": map[string]any{
			"consent_required": true, "redirect_uri": body["post_logout_redirect_uri"],
		}})
	default:
		s.json(r, 404, map[string]any{"error_type": "not_found", "error_message": "the stub IdP doesn't answer " + path})
	}
}

func (s *Session) json(r playwright.Route, status int, body map[string]any) {
	body["status_code"] = status
	body["request_id"] = "request-id-test"
	b, err := json.Marshal(body)
	if err != nil {
		_ = r.Abort()
		return
	}
	_ = r.Fulfill(playwright.RouteFulfillOptions{
		Status: playwright.Int(status), ContentType: playwright.String("application/json"), Body: b, Headers: s.cors(),
	})
}

func (s *Session) cors() map[string]string {
	return map[string]string{
		"access-control-allow-origin":      s.origin,
		"access-control-allow-credentials": "true",
	}
}

var stubBootstrap = map[string]any{
	"project_name":            "MyKeebs",
	"captcha_settings":        map[string]any{"enabled": false},
	"disable_sdk_watermark":   false,
	"email_domains":           []string{},
	"pkce_required_for_oauth": false,
	"rbac_policy":             map[string]any{"resources": []any{}, "roles": []any{}, "scopes": []any{}},
	"vertical":                "CONSUMER",
}

var stubSession = map[string]any{
	"session_id":             "session-test-1",
	"user_id":                "user-test-1",
	"started_at":             "2026-01-01T00:00:00Z",
	"last_accessed_at":       "2026-01-01T00:00:00Z",
	"expires_at":             "2099-01-01T00:00:00Z",
	"attributes":             map[string]any{"ip_address": "", "user_agent": ""},
	"authentication_factors": []any{},
	"custom_claims":          map[string]any{},
	"roles":                  []string{"stytch_user"},
}

var stubUser = map[string]any{
	"user_id":   "user-test-1",
	"name":      map[string]any{"first_name": "Test", "middle_name": "", "last_name": "User"},
	"emails":    []any{},
	"providers": []map[string]any{{"provider_type": "Discord"}},
	"roles":     []string{"stytch_user"},
	"status":    "active",
}

var stubAuthenticated = map[string]any{
	"session":       stubSession,
	"session_token": "session-token-test",
	"session_jwt":   "session-jwt-test",
	"user":          stubUser,
}
