// Package consent serves the IdP consent/authorization page at GET /authorize,
// which also handles OIDC logout requests, so MCP clients can authenticate
// against kbdb standalone without depending on mykeebs-web being deployed. The
// page itself is built in github.com/roguekeyboards/mykeebs-consent.
package consent

import (
	"log"
	"net/http"

	consentpage "github.com/roguekeyboards/mykeebs-consent"
)

// Handler serves GET /authorize: kbdb's IdP consent, login and logout page.
func Handler(idpConsentPublicToken, oidcIssuerBaseURL string) http.Handler {
	page, err := consentpage.Page(consentpage.Config{
		PublicToken:   idpConsentPublicToken,
		IssuerBaseURL: oidcIssuerBaseURL,
	})
	if err != nil {
		log.Fatalf("consent: rendering the page: %v", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
}
