// Package brand holds mykeebs' logo, icons and design tokens for Go code
// that serves pages (the consent page), so it embeds the
// same artwork and colours the web app uses rather than copying them.
//
// The source of truth is tokens.json (colours and font stacks) plus the
// designs in internal/art; dist/ is generated from them by `mise run gen`
// and also published to npm as @rogueserenity/mykeebs-brand for mykeebs-web.
// See README.md for which file is meant for what.
package brand

import (
	"embed"
	"io/fs"

	"github.com/rogueserenity/kbdb/brand/internal/tokens"
)

type (
	// Token is one named value from tokens.json, e.g. {"accent", "#e8a33d"}.
	Token = tokens.Token
	// Palette is one section of tokens.json, in file order.
	Palette = tokens.Palette
	// TokenSet is the whole of tokens.json.
	TokenSet = tokens.Tokens
)

//go:embed tokens.json
var tokensJSON []byte

//go:embed dist
var dist embed.FS

// Files holds the generated assets by file name, e.g. "mark-on-dark.svg",
// "favicon.svg", "tokens.css".
var Files fs.FS = mustSub(dist, "dist")

// Tokens is tokens.json: Tokens.Theme for page colours and fonts,
// Tokens.Logo for the logo palette.
var Tokens = mustParse(tokensJSON)

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func mustParse(b []byte) TokenSet {
	t, err := tokens.Parse(b)
	if err != nil {
		panic(err)
	}
	return t
}
