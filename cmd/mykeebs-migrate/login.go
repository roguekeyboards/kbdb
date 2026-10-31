package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// redirectPath is the path component of the registered localhost redirect URI
// (http://localhost:8765/authorize.html). The host+port+path must match a
// redirect URI registered on the IdP exactly.
const redirectPath = "/authorize.html"

// oidcMetadata is the subset of the issuer's discovery document we use.
type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

// Run executes the browser login flow and caches the resulting access token.
func (c *LoginCmd) Run(ctx context.Context) error {
	redirectURI := fmt.Sprintf("http://localhost:%d%s", c.Port, redirectPath)

	meta, err := discoverOIDC(ctx, c.Issuer)
	if err != nil {
		return err
	}

	existing, _, err := loadCreds(c.Issuer)
	if err != nil {
		return err
	}

	clientID := c.ClientID
	if clientID == "" {
		clientID = existing.ClientID
	}
	if clientID == "" {
		if meta.RegistrationEndpoint == "" {
			return errors.New("no --client-id given and the issuer advertises no registration_endpoint; provision a client and pass --client-id (or set MYKEEBS_OIDC_CLIENT_ID)")
		}
		clientID, err = registerClient(ctx, meta.RegistrationEndpoint, redirectURI)
		if err != nil {
			return err
		}
		fmt.Printf("registered a new OAuth client: %s\n", clientID)
	}

	verifier, challenge := newPKCEPair()
	state, err := randomURLSafe(24)
	if err != nil {
		return err
	}

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf("localhost:%d", c.Port))
	if err != nil {
		return fmt.Errorf("cannot bind localhost:%d for the login redirect (is it in use?): %w", c.Port, err)
	}
	defer func() { _ = ln.Close() }()

	authURL := buildAuthURL(meta.AuthorizationEndpoint, clientID, redirectURI, challenge, state)
	fmt.Println("Opening your browser to sign in. If it doesn't open, visit:")
	fmt.Println("  " + authURL)
	openBrowser(ctx, authURL)

	code, err := waitForCode(ctx, ln, state)
	if err != nil {
		return err
	}

	tok, err := exchangeCode(ctx, meta.TokenEndpoint, clientID, redirectURI, code, verifier)
	if err != nil {
		return err
	}

	sub, err := tokenSubject(tok.AccessToken)
	if err != nil {
		return fmt.Errorf("the token endpoint returned a token we can't read: %w", err)
	}

	creds := cachedCreds{
		AccessToken: tok.AccessToken,
		Expiry:      time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
		ClientID:    clientID,
	}
	if err := saveCreds(c.Issuer, creds); err != nil {
		return err
	}

	fmt.Printf("logged in as %s; token cached (expires %s)\n", sub, creds.Expiry.Format(time.RFC3339))
	fmt.Println(tok.AccessToken)
	return nil
}

// discoverOIDC resolves the issuer's endpoints. It reads RFC 8414
// Authorization Server Metadata (/.well-known/oauth-authorization-server)
// first, since that is where this IdP advertises registration_endpoint, and
// falls back to the OIDC document (/.well-known/openid-configuration). Fields
// missing from the primary doc are backfilled from the fallback.
func discoverOIDC(ctx context.Context, issuer string) (oidcMetadata, error) {
	base := strings.TrimRight(issuer, "/")
	primary, errPrimary := fetchMetadata(ctx, base+"/.well-known/oauth-authorization-server")
	fallback, errFallback := fetchMetadata(ctx, base+"/.well-known/openid-configuration")

	if errPrimary != nil && errFallback != nil {
		return oidcMetadata{}, fmt.Errorf("no usable discovery document for %s: %w", issuer, errPrimary)
	}

	meta := primary
	if errPrimary != nil {
		meta = fallback
	}
	if meta.AuthorizationEndpoint == "" {
		meta.AuthorizationEndpoint = fallback.AuthorizationEndpoint
	}
	if meta.TokenEndpoint == "" {
		meta.TokenEndpoint = fallback.TokenEndpoint
	}
	if meta.RegistrationEndpoint == "" {
		meta.RegistrationEndpoint = fallback.RegistrationEndpoint
	}

	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return oidcMetadata{}, fmt.Errorf("discovery for %s is missing authorization/token endpoints", issuer)
	}
	return meta, nil
}

// fetchMetadata GETs one metadata URL and decodes it.
func fetchMetadata(ctx context.Context, metadataURL string) (oidcMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return oidcMetadata{}, fmt.Errorf("building request for %s: %w", metadataURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcMetadata{}, fmt.Errorf("fetching %s: %w", metadataURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return oidcMetadata{}, fmt.Errorf("%s returned HTTP %d", metadataURL, resp.StatusCode)
	}
	var meta oidcMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return oidcMetadata{}, fmt.Errorf("decoding %s: %w", metadataURL, err)
	}
	return meta, nil
}

// registerClient does an RFC 7591 dynamic client registration for a native app
// (public client, auth-code grant) and returns the issued client_id.
func registerClient(ctx context.Context, registrationEndpoint, redirectURI string) (string, error) {
	body := map[string]any{
		"client_name":                "mykeebs-migrate CLI",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encoding registration request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("building registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling registration endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registration endpoint returned HTTP %d", resp.StatusCode)
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding registration response: %w", err)
	}
	if out.ClientID == "" {
		return "", errors.New("registration response had no client_id")
	}
	return out.ClientID, nil
}

// newPKCEPair returns a verifier and its S256 challenge.
func newPKCEPair() (verifier, challenge string) {
	v, _ := randomURLSafe(48)
	sum := sha256.Sum256([]byte(v))
	return v, base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomURLSafe returns n random bytes, base64url without padding.
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading randomness: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// buildAuthURL assembles the authorization request URL.
func buildAuthURL(authEndpoint, clientID, redirectURI, challenge, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "openid profile email")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	return authEndpoint + "?" + q.Encode()
}

// waitForCode serves the single redirect on ln, validates state, and returns
// the authorization code. It also opens the system browser at the auth URL —
// callers print the URL first as a fallback.
func waitForCode(ctx context.Context, ln net.Listener, wantState string) (string, error) {
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)

	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != redirectPath {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			if e := q.Get("error"); e != "" {
				writeBrowserMessage(w, "Login failed: "+e)
				done <- result{err: fmt.Errorf("authorization error: %s: %s", e, q.Get("error_description"))}
				return
			}
			if q.Get("state") != wantState {
				writeBrowserMessage(w, "Login failed: state mismatch.")
				done <- result{err: errors.New("state parameter mismatch (possible CSRF); aborting")}
				return
			}
			code := q.Get("code")
			if code == "" {
				writeBrowserMessage(w, "Login failed: no authorization code.")
				done <- result{err: errors.New("redirect carried no authorization code")}
				return
			}
			writeBrowserMessage(w, "Signed in. You can close this tab and return to the terminal.")
			done <- result{code: code}
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	//nolint:contextcheck // shutdown deliberately uses a fresh ctx so it can drain after the caller's ctx is cancelled.
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-done:
		return res.code, res.err
	case <-time.After(5 * time.Minute):
		return "", errors.New("timed out waiting for the browser redirect")
	}
}

// browserPage renders a fixed one-line status page. msg is escaped before
// interpolation so it is safe even though it currently only carries our own
// strings.
var browserPage = template.Must(template.New("page").Parse(
	`<!doctype html><meta charset=utf-8><title>mykeebs-migrate</title>` +
		`<body style="font-family:system-ui;margin:4rem auto;max-width:32rem"><p>{{.}}</p></body>`))

func writeBrowserMessage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = browserPage.Execute(w, msg)
}

// tokenResponse is the token endpoint's success body.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// exchangeCode swaps an authorization code for tokens using PKCE (no client
// secret; this is a public native client).
func exchangeCode(ctx context.Context, tokenEndpoint, clientID, redirectURI, code, verifier string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("calling token endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var tok tokenResponse
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return tokenResponse{}, fmt.Errorf("decoding token response: %w", err)
	}
	if tok.AccessToken == "" {
		return tokenResponse{}, errors.New("token response had no access_token")
	}
	return tok, nil
}

// openBrowser best-effort opens url in the system browser. Failures are
// ignored: the caller always prints the URL as a fallback.
func openBrowser(ctx context.Context, url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	// name is a fixed per-OS launcher; args is only the auth URL we built.
	_ = exec.CommandContext(ctx, name, args...).Start() //nolint:gosec // fixed launcher, our own URL.
}
