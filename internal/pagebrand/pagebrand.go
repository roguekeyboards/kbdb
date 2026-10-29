// Package pagebrand holds the mykeebs artwork kbdb's own HTML pages (the
// consent page, the privacy policy and terms, the MCP guide and the AI
// statement) embed inline, so they render without depending on the web app
// being up.
package pagebrand

import (
	"encoding/base64"
	"html/template"
	"io/fs"

	"github.com/rogueserenity/kbdb/brand"
)

// Brand is the artwork a page embeds. Its values are trusted: they come from
// brand's embedded files, never a request.
type Brand struct {
	Logo    template.HTML
	Favicon template.URL
	Tokens  template.CSS
}

// Assets is the mykeebs logo, favicon and theme tokens.
var Assets = Brand{
	Logo:    template.HTML(mustReadBrandFile("logo-horizontal-on-dark.svg")),                                                  //nolint:gosec // embedded brand file.
	Favicon: template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(mustReadBrandFile("favicon.svg"))), //nolint:gosec // embedded brand file.
	Tokens:  template.CSS(mustReadBrandFile("tokens.css")),                                                                    //nolint:gosec // embedded brand file.
}

func mustReadBrandFile(name string) []byte {
	b, err := fs.ReadFile(brand.Files, name)
	if err != nil {
		panic(err)
	}
	return b
}
