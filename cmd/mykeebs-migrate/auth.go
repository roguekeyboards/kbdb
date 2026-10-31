package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tokenSubject returns the "sub" claim of a JWT, decoded without signature
// verification. The caller already obtained the token from a trusted issuer;
// this only reads its subject back out to fill {userId} path segments. Mirrors
// test/functional/support/api.TokenSubject.
func tokenSubject(rawToken string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed token: expected 3 dot-separated parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding token payload: %w", err)
	}

	var claims struct {
		Subject string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("decoding token claims: %w", err)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("token has no sub claim")
	}
	return claims.Subject, nil
}

// cachedCreds is what a login writes and dump/restore/verify read. It is keyed
// on disk by the issuer host so tokens for different environments don't
// collide. Storing the bearer token on disk (0600) is the intended behaviour,
// mirroring gh/aws CLI credential caches.
type cachedCreds struct {
	AccessToken string    `json:"access_token"` //nolint:gosec // this file is the token cache; persisting it is the point.
	Expiry      time.Time `json:"expiry"`
	// ClientID is filled only when login registered one dynamically, so a
	// later login for the same issuer can reuse it.
	ClientID string `json:"client_id,omitempty"`
}

// configDir is ~/.config/mykeebs-migrate (or $XDG_CONFIG_HOME/mykeebs-migrate),
// created if absent.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	dir := filepath.Join(base, "mykeebs-migrate")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

// credsPath is the cache file for a given issuer.
func credsPath(issuer string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	host := issuer
	if u, err := url.Parse(issuer); err == nil && u.Host != "" {
		host = u.Host
	}
	safe := strings.NewReplacer("/", "_", ":", "_").Replace(host)
	return filepath.Join(dir, "token-"+safe+".json"), nil
}

// loadCreds reads the cache for issuer. found is false when there is no cache
// file yet; an unreadable or malformed file is an error.
func loadCreds(issuer string) (creds cachedCreds, found bool, err error) {
	path, err := credsPath(issuer)
	if err != nil {
		return cachedCreds{}, false, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the issuer, not caller input.
	if os.IsNotExist(err) {
		return cachedCreds{}, false, nil
	}
	if err != nil {
		return cachedCreds{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return cachedCreds{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	return creds, true, nil
}

// saveCreds writes the cache for issuer with 0600 permissions.
func saveCreds(issuer string, creds cachedCreds) error {
	path, err := credsPath(issuer)
	if err != nil {
		return err
	}
	// This file is deliberately the on-disk token cache (0600), like the
	// gh/aws CLI credential stores.
	data, err := json.MarshalIndent(creds, "", "  ") //nolint:gosec // persisting the token here is the purpose.
	if err != nil {
		return fmt.Errorf("encoding creds: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// resolveToken picks the bearer token for a dump/restore/verify run: an
// explicit --token wins; otherwise the unexpired login cache for issuer is
// used. issuer may be empty only when explicitToken is set.
func resolveToken(explicitToken, issuer string) (string, error) {
	if explicitToken != "" {
		return explicitToken, nil
	}
	if issuer == "" {
		return "", fmt.Errorf("no --token given and no --issuer to look up a cached login; pass one of them")
	}
	creds, found, err := loadCreds(issuer)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no cached login for %s; run 'mykeebs-migrate login --issuer %s' or pass --token", issuer, issuer)
	}
	if !creds.Expiry.IsZero() && time.Now().After(creds.Expiry) {
		return "", fmt.Errorf("cached login for %s expired at %s; run 'mykeebs-migrate login' again", issuer, creds.Expiry.Format(time.RFC3339))
	}
	return creds.AccessToken, nil
}
