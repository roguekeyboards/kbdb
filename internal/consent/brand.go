package consent

import (
	"encoding/base64"
	"html/template"
	"io/fs"

	"github.com/rogueserenity/kbdb/brand"
)

// pageBrand is the mykeebs artwork the consent and logout pages embed
// inline, so they render without depending on the web app being up. Its
// values are trusted: they come from brand's embedded files, never a request.
type pageBrand struct {
	Logo    template.HTML
	Favicon template.URL
	Tokens  template.CSS
}

var pageBrandAssets = pageBrand{
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
