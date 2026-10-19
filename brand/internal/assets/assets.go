// Package assets lists every file in brand/dist and builds it from
// tokens.json and the vendored font. The gen command writes its output; the
// package's tests check the committed dist/ still matches it.
package assets

import (
	"fmt"
	"strings"

	"github.com/rogueserenity/kbdb/brand/internal/art"
	"github.com/rogueserenity/kbdb/brand/internal/tokens"
)

// Paths within the brand directory.
const (
	TokensFile = "tokens.json"
	FontFile   = "fonts/JetBrainsMono-ExtraBold.ttf"
	DistDir    = "dist"
)

// Build renders every asset, keyed by its file name under dist/.
func Build(tokensJSON, font []byte) (map[string][]byte, error) {
	tok, err := tokens.Parse(tokensJSON)
	if err != nil {
		return nil, err
	}
	c, err := art.ColorsFrom(tok.Logo)
	if err != nil {
		return nil, err
	}
	f, err := art.LoadFont(font)
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{"tokens.css": tokensCSS(tok)}

	svgs := map[string]func() (art.Scene, error){
		"logo-horizontal-on-dark.svg":  func() (art.Scene, error) { return art.Horizontal(f, c.OnDark()) },
		"logo-horizontal-on-light.svg": func() (art.Scene, error) { return art.Horizontal(f, c.OnLight()) },
		"logo-stacked-on-dark.svg":     func() (art.Scene, error) { return art.Stacked(f, c.OnDark()) },
		"logo-stacked-on-light.svg":    func() (art.Scene, error) { return art.Stacked(f, c.OnLight()) },
		"mark-on-dark.svg":             func() (art.Scene, error) { return art.Mark(c.OnDark()), nil },
		"mark-on-light.svg":            func() (art.Scene, error) { return art.Mark(c.OnLight()), nil },
		"favicon.svg":                  func() (art.Scene, error) { return art.Favicon32(c), nil },
		"discord-avatar.svg":           func() (art.Scene, error) { return art.Tile(c, art.DiscordFrac, "mykeebs Discord avatar"), nil },
		"discord-avatar-dev.svg":       func() (art.Scene, error) { return art.DevAvatar(f, c) },
	}
	for name, mk := range svgs {
		sc, err := mk()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = sc.SVG()
	}

	dev, err := art.DevAvatar(f, c)
	if err != nil {
		return nil, err
	}
	appIcon := art.Tile(c, art.AppIconFrac, "mykeebs")
	pngs := []struct {
		name  string
		scene art.Scene
		px    int
	}{
		{"favicon-16.png", art.Favicon16(c), 16},
		{"favicon-32.png", art.Favicon32(c), 32},
		{"favicon-48.png", art.Favicon32(c), 48},
		{"apple-touch-icon.png", appIcon, 180},
		{"icon-192.png", appIcon, 192},
		{"icon-512.png", appIcon, 512},
		{"discord-avatar.png", art.Tile(c, art.DiscordFrac, "mykeebs Discord avatar"), 1024},
		{"discord-avatar-dev.png", dev, 1024},
	}
	for _, p := range pngs {
		data, err := p.scene.PNG(p.px)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.name, err)
		}
		files[p.name] = data
	}

	ico, err := art.ICO(files["favicon-16.png"], files["favicon-32.png"], files["favicon-48.png"])
	if err != nil {
		return nil, fmt.Errorf("favicon.ico: %w", err)
	}
	files["favicon.ico"] = ico
	delete(files, "favicon-48.png") // only needed inside the .ico
	return files, nil
}

// tokensCSS renders the tokens as CSS custom properties: theme tokens under
// their own names (--accent), logo colours prefixed (--logo-amber).
func tokensCSS(t tokens.Tokens) []byte {
	var b strings.Builder
	b.WriteString("/* Generated from brand/tokens.json by `mise run gen`. Do not edit. */\n:root {\n")
	for _, tk := range t.Theme {
		fmt.Fprintf(&b, "\t--%s: %s;\n", tk.Name, tk.Value)
	}
	b.WriteString("\n")
	for _, tk := range t.Logo {
		fmt.Fprintf(&b, "\t--logo-%s: %s;\n", tk.Name, tk.Value)
	}
	b.WriteString("}\n")
	return []byte(b.String())
}
