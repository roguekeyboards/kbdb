// Package help serves the guide to connecting an AI assistant to mykeebs
// over MCP at GET /help/mcp. It's served by kbdb rather than mykeebs-web so
// it names the MCP endpoint of whichever deployment serves it, and so the
// guide changes in the same place the tools do.
package help

import (
	_ "embed"
	"html/template"
	"log"
	"net/http"

	"github.com/rogueserenity/kbdb/internal/pagebrand"
)

//go:embed mcp.html
var mcpHTML string

var mcpTemplate = template.Must(template.New("mcp.html").Parse(mcpHTML))

type templateData struct {
	pagebrand.Brand
	MCPURL string
}

// MCPHandler serves GET /help/mcp.
func MCPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := templateData{Brand: pagebrand.Assets, MCPURL: schemeOf(r) + "://" + r.Host + "/mcp"}
		if err := mcpTemplate.Execute(w, data); err != nil {
			log.Printf("help: rendering mcp.html: %v", err)
		}
	})
}

func schemeOf(r *http.Request) string {
	if fp := r.Header.Get("X-Forwarded-Proto"); fp != "" {
		return fp
	}
	return "https"
}
