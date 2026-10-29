// Package tokens parses brand/tokens.json, the single source of mykeebs'
// colours and fonts. It's internal so the brand package (which embeds the
// generated assets) and the generator (which writes them) can share one
// parser without the generator depending on its own output.
package tokens

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Token is one named value, e.g. {"accent", "#e8a33d"}. Names are the CSS
// custom property names without the leading "--".
type Token struct {
	Name  string
	Value string
}

// Palette is a group of tokens in the order tokens.json lists them, so
// generated output (tokens.css) keeps the file's grouping instead of
// whatever order a Go map would iterate in.
type Palette []Token

// Lookup returns the value of the named token.
func (p Palette) Lookup(name string) (string, bool) {
	for _, t := range p {
		if t.Name == name {
			return t.Value, true
		}
	}
	return "", false
}

// UnmarshalJSON reads a flat JSON object of string values, keeping key order.
func (p *Palette) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return errors.New("palette must be a JSON object")
	}
	var out Palette
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		name, _ := tok.(string)
		var value string
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("token %q: value must be a string: %w", name, err)
		}
		if value == "" {
			return fmt.Errorf("token %q is empty", name)
		}
		if _, dup := out.Lookup(name); dup {
			return fmt.Errorf("token %q is listed twice", name)
		}
		out = append(out, Token{Name: name, Value: value})
	}
	*p = out
	return nil
}

// Tokens is the parsed tokens.json.
type Tokens struct {
	// Logo holds the colours the logo artwork is drawn in.
	Logo Palette `json:"logo"`
	// Theme holds the UI theme: colours and font stacks shared by every
	// page mykeebs serves (the web app, mykeebs.com, kbdb's sign-in page).
	Theme Palette `json:"theme"`
}

// Parse reads tokens.json.
func Parse(b []byte) (Tokens, error) {
	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		return Tokens{}, fmt.Errorf("parsing tokens.json: %w", err)
	}
	if len(t.Logo) == 0 || len(t.Theme) == 0 {
		return Tokens{}, errors.New("tokens.json needs both a logo and a theme section")
	}
	return t, nil
}
