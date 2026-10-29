// Package about serves the statement on how AI is used to build mykeebs at
// GET /about/ai. It covers both kbdb and mykeebs-web, and is served by kbdb
// alongside the privacy policy and terms so it stays reachable when the web
// app isn't.
package about

import (
	"bytes"
	_ "embed"
	"html/template"
	"log"
	"net/http"

	"github.com/rogueserenity/kbdb/internal/pagebrand"
)

//go:embed ai.html
var aiHTML string

// AIHandler serves GET /about/ai.
func AIHandler() http.Handler {
	tmpl := template.Must(template.New("ai.html").Parse(aiHTML))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, pagebrand.Assets); err != nil {
		log.Fatalf("about: rendering ai.html: %v", err)
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
