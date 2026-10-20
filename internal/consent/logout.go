package consent

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"

	"github.com/rogueserenity/kbdb/internal/pagebrand"
	"github.com/rogueserenity/kbdb/internal/problem"
)

//go:embed logout.html
var logoutHTML string

var logoutTemplate = template.Must(template.New("logout.html").Parse(logoutHTML))

// logoutTemplateData fields are rendered by html/template into a <script>
// block - its context-aware escaping renders them as safe JS string
// literals (including escaping a </script> breakout attempt from ReturnTo,
// which is user-controlled via the return_to query param), so fields are
// passed raw, not pre-quoted.
type logoutTemplateData struct {
	IDPConsentPublicToken string
	OIDCIssuerBaseURL     string
	ReturnTo              string
	Brand                 pagebrand.Brand
}

// LogoutHandler serves GET /logout: revokes the IdP session on this
// origin (see internal/consent's package doc for why that can't be done
// from mykeebs-web's own origin), then redirects to return_to.
// allowedReturnOrigins restricts return_to to known callers, since it would
// otherwise be an open redirect.
func LogoutHandler(idpConsentPublicToken, oidcIssuerBaseURL string, allowedReturnOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		returnTo, err := validReturnTo(r.URL.Query().Get("return_to"), allowedReturnOrigins)
		if err != nil {
			problem.BadRequest(w, err.Error())
			return
		}

		data := logoutTemplateData{
			IDPConsentPublicToken: idpConsentPublicToken,
			OIDCIssuerBaseURL:     oidcIssuerBaseURL,
			ReturnTo:              returnTo,
			Brand:                 pagebrand.Assets,
		}

		var buf bytes.Buffer
		if err := logoutTemplate.Execute(&buf, data); err != nil {
			log.Fatalf("consent: rendering logout.html: %v", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	})
}

// validReturnTo rejects a return_to whose origin isn't in allowedOrigins.
func validReturnTo(returnTo string, allowedOrigins []string) (string, error) {
	if returnTo == "" {
		return "", fmt.Errorf("return_to is required")
	}

	parsed, err := url.Parse(returnTo)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("return_to must be an absolute URL")
	}

	origin := parsed.Scheme + "://" + parsed.Host
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return returnTo, nil
		}
	}

	return "", fmt.Errorf("return_to origin %q is not allowed", origin)
}
