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
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/roguekeyboards/kbdb/internal/pagebrand"
)

//go:embed authorize.html
var authorizeHTML string

var authorizeTemplate = template.Must(template.New("authorize.html").Parse(authorizeHTML))

// idpScript is the IdP's browser SDK, unmodified, at the version pinned in
// package-lock.json: for @stytch/vanilla-js, the npm package's
// dist/iife/index.js. scripts/idp-sdk.sh copies it in; Renovate bumps it.
//
//go:embed idp-sdk.js
var idpScript []byte

var idpScriptSum = sha512.Sum384(idpScript)

// IDPScriptPath is where IDPScriptHandler is served. It names the file by
// its hash, so browsers can cache it for good and a new SDK gets a new URL.
// The page links to it by a relative URL, so it also loads behind a
// path prefix such as floci's stage path.
var IDPScriptPath = "/authorize/idp-sdk-" + hex.EncodeToString(idpScriptSum[:8]) + ".js"

var idpScriptIntegrity = "sha384-" + base64.StdEncoding.EncodeToString(idpScriptSum[:])

// templateData fields are passed raw, not pre-quoted: html/template's
// context-aware escaping renders IDPConsentPublicToken and OIDCIssuerBaseURL
// as safe JS string literals in the <script> block (including escaping a
// </script> breakout attempt), and the IDPScript fields as attribute values.
type templateData struct {
	IDPConsentPublicToken string
	OIDCIssuerBaseURL     string
	IDPScriptSrc          string
	IDPScriptIntegrity    string
	Brand                 pagebrand.Brand
}

// Handler serves GET /authorize: kbdb's IdP consent, login and logout page.
func Handler(idpConsentPublicToken, oidcIssuerBaseURL string) http.Handler {
	var buf bytes.Buffer
	data := templateData{
		IDPConsentPublicToken: idpConsentPublicToken,
		OIDCIssuerBaseURL:     oidcIssuerBaseURL,
		IDPScriptSrc:          strings.TrimPrefix(IDPScriptPath, "/"),
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
