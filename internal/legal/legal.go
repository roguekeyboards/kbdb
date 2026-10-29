// Package legal serves the privacy policy at GET /privacy and the terms of
// service at GET /terms. They're served by kbdb rather than mykeebs-web
// because they cover the whole system, MCP and REST included, and must stay
// reachable when the web app isn't.
package legal

import (
	"bytes"
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/rogueserenity/kbdb/internal/pagebrand"
)

//go:embed *.html
var pages embed.FS

// PrivacyHandler serves GET /privacy.
func PrivacyHandler() http.Handler { return handler("privacy.html") }

// TermsHandler serves GET /terms.
func TermsHandler() http.Handler { return handler("terms.html") }

func handler(content string) http.Handler {
	tmpl := template.Must(template.ParseFS(pages, "layout.html", content))
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", pagebrand.Assets); err != nil {
		log.Fatalf("legal: rendering %s: %v", content, err)
	}
	page := buf.Bytes()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
}
