package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"time"
)

// impersonationPath is where the Stytch dashboard's "Impersonate user" sends
// its token: the path of the project's default Login redirect URL.
const impersonationPath = "/impersonate"

// sessionPath is where the impersonation page posts the session it got.
const sessionPath = "/impersonate/session"

// impersonationPage swaps the token in the redirect for a session JWT. Only
// Stytch's browser SDK can do that without the project secret, and only from
// a page on one of the project's authorized SDK domains. It loads the SDK
// from jsDelivr rather than a copy kept here, and revokes the session once it
// has the JWT: kbdb checks the JWT itself, which still works until it
// expires.
var impersonationPage = template.Must(template.New("impersonate").Parse(`<!doctype html>
<meta charset=utf-8><title>mykeebs-migrate</title>
<body style="font-family:system-ui;margin:4rem auto;max-width:32rem"><p id=msg>Exchanging the impersonation token...</p>
<script type=module>
const msg = document.getElementById('msg');
const result = { nonce: {{.Nonce}} };
try {
  const token = new URLSearchParams(location.search).get('token');
  if (!token) throw new Error('the redirect carried no token');
  const { createStytchClient } = await import('https://cdn.jsdelivr.net/npm/@stytch/vanilla-js@6/+esm');
  const stytch = createStytchClient({{.PublicToken}}, { customBaseUrl: {{.Issuer}} });
  const resp = await stytch.impersonation.authenticate({ impersonation_token: token });
  result.session_jwt = resp.session_jwt;
  await stytch.session.revoke().catch(() => {});
} catch (e) {
  result.error = String(e?.message ?? e);
}
const ack = await fetch({{.SessionPath}}, { method: 'POST', body: JSON.stringify(result) });
msg.textContent = result.error ? 'Failed: ' + result.error
  : ack.ok ? 'Done. You can close this tab and return to the terminal.'
  : 'The terminal rejected the session.';
</script>`))

// impersonate waits on localhost:port for the redirect a Stytch dashboard
// impersonation opens, and returns the session JWT the page exchanged it for.
func impersonate(ctx context.Context, issuer, publicToken string, port int) (string, error) {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return "", fmt.Errorf("cannot bind localhost:%d for the impersonation redirect (is it in use?): %w", port, err)
	}
	defer func() { _ = ln.Close() }()

	fmt.Printf("Waiting for an impersonation on http://localhost:%d%s.\n", port, impersonationPath)
	fmt.Println("In the Stytch dashboard, turn on Allow user impersonation, open the user,")
	fmt.Println("and click Impersonate user, then Launch in new tab.")

	jwt, err := waitForSession(ctx, ln, issuer, publicToken)
	if err != nil {
		return "", err
	}
	sub, err := tokenSubject(jwt)
	if err != nil {
		return "", fmt.Errorf("the impersonation returned a token we can't read: %w", err)
	}
	fmt.Printf("impersonating %s\n\n", sub)
	return jwt, nil
}

// waitForSession serves the impersonation page on ln and returns the session
// JWT it posts back. The post must carry the nonce the page was rendered
// with, so another page in the browser can't hand over a token of its own.
func waitForSession(ctx context.Context, ln net.Listener, issuer, publicToken string) (string, error) {
	nonce, err := randomURLSafe(24)
	if err != nil {
		return "", err
	}
	type result struct {
		jwt string
		err error
	}
	done := make(chan result, 1)
	// A second post (a reloaded tab) is refused rather than blocking.
	send := func(r result) bool {
		select {
		case done <- r:
			return true
		default:
			return false
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+impersonationPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = impersonationPage.Execute(w, map[string]string{
			"Nonce":       nonce,
			"PublicToken": publicToken,
			"Issuer":      issuer,
			"SessionPath": sessionPath,
		})
	})
	mux.HandleFunc("POST "+sessionPath, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Nonce      string `json:"nonce"`
			SessionJWT string `json:"session_jwt"`
			Error      string `json:"error"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Nonce != nonce {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var res result
		switch {
		case body.Error != "":
			res.err = fmt.Errorf("exchanging the impersonation token: %s", body.Error)
		case body.SessionJWT == "":
			res.err = errors.New("the impersonation returned no session JWT")
		default:
			res.jwt = body.SessionJWT
		}
		if !send(res) {
			http.Error(w, "already have a result", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: mux}
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
		return res.jwt, res.err
	case <-time.After(5 * time.Minute):
		return "", errors.New("timed out waiting for the impersonation redirect")
	}
}
