// Package consent hosts the IdP consent/authorization page kbdb serves
// at GET /authorize, which also handles OIDC logout requests, so MCP
// clients can authenticate against kbdb standalone without depending on
// mykeebs-web being deployed. The embedded HTML wires up whatever IdP
// browser SDK the deployment uses.
package consent

import (
	"bytes"
	"crypto/sha512"
	_ "embed"
	"encoding/base64"
	"html/template"
	"log"
	"net/http"

	"github.com/rogueserenity/kbdb/internal/pagebrand"
)

//go:embed authorize.html
var authorizeHTML string

var authorizeTemplate = template.Must(template.New("authorize.html").Parse(authorizeHTML))

// idpScript is the IdP's browser SDK, unmodified: for @stytch/vanilla-js,
// the npm package's dist/iife/index.js. To upgrade, replace the file and
// put the new version in its name and IDPScriptPath.
//
//go:embed stytch-vanilla-js-6.2.0.js
var idpScript []byte

// IDPScriptPath is where IDPScriptHandler is served. The version in
// the name lets browsers cache it for good.
const IDPScriptPath = "/authorize/stytch-vanilla-js-6.2.0.js"

var idpScriptIntegrity = func() string {
	sum := sha512.Sum384(idpScript)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}()

// templateData fields are rendered by html/template into a <script> block -
// its context-aware escaping renders them as safe JS string literals
// (including escaping a </script> breakout attempt), so fields are passed
// raw, not pre-quoted.
type templateData struct {
	IDPConsentPublicToken string
	OIDCIssuerBaseURL     string
	IDPScriptPath         string
	IDPScriptIntegrity    string
	Brand                 pagebrand.Brand
}

// Handler serves GET /authorize: kbdb's IdP consent, login and logout page.
func Handler(idpConsentPublicToken, oidcIssuerBaseURL string) http.Handler {
	var buf bytes.Buffer
	data := templateData{
		IDPConsentPublicToken: idpConsentPublicToken,
		OIDCIssuerBaseURL:     oidcIssuerBaseURL,
		IDPScriptPath:         IDPScriptPath,
		IDPScriptIntegrity:    idpScriptIntegrity,
		Brand:                 pagebrand.Assets,
	}
	if err := authorizeTemplate.Execute(&buf, data); err != nil {
		log.Fatalf("consent: rendering authorize.html: %v", err)
	}
	page := buf.Bytes()

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

// IDPScriptHandler serves GET IDPScriptPath: the IdP browser SDK the
// consent page loads.
func IDPScriptHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(idpScript)
	})
}
