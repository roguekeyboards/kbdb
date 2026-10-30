// Package pagebrand holds the mykeebs artwork kbdb's consent page embeds
// inline, so it renders without depending on the web app being up.
package pagebrand

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"strings"

	"github.com/roguekeyboards/kbdb/brand"
)

// fontBaseURL serves the brand fonts for dev and prod alike. Files under a
// version never change; new files go under a new version.
const fontBaseURL = "https://mykeebs.com/fonts/v1/"

// Brand is the artwork a page embeds. Its values are trusted: they come from
// brand's embedded files, never a request.
type Brand struct {
	Logo    template.HTML
	Favicon template.URL
	Tokens  template.CSS
	Fonts   template.CSS
}

// Assets is the mykeebs logo, favicon, theme tokens and font faces.
var Assets = Brand{
	Logo:    template.HTML(mustReadBrandFile("logo-horizontal-on-dark.svg")),                                                  //nolint:gosec // embedded brand file.
	Favicon: template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(mustReadBrandFile("favicon.svg"))), //nolint:gosec // embedded brand file.
	Tokens:  template.CSS(mustReadBrandFile("tokens.css")),                                                                    //nolint:gosec // embedded brand file.
	Fonts: fontFaces([]fontFamily{
		{Name: "JetBrains Mono", File: "jetbrains-mono", Weights: []int{400, 500, 600, 700}},
		{Name: "Inter", File: "inter", Weights: []int{400, 500, 600}},
	}),
}

type fontFamily struct {
	Name    string
	File    string
	Weights []int
}

// fontFaces swaps in system fonts while the files load, and keeps them if
// the files can't load.
func fontFaces(families []fontFamily) template.CSS {
	var b strings.Builder
	for _, f := range families {
		for _, w := range f.Weights {
			fmt.Fprintf(&b, "@font-face { font-family: '%s'; font-style: normal; font-weight: %d; font-display: swap; src: url('%s%s-latin-%d.woff2') format('woff2'); }\n",
				f.Name, w, fontBaseURL, f.File, w)
		}
	}
	return template.CSS(b.String()) //nolint:gosec // built from the constants above.
}

func mustReadBrandFile(name string) []byte {
	b, err := fs.ReadFile(brand.Files, name)
	if err != nil {
		panic(err)
	}
	return b
}
