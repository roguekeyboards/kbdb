// Command gen regenerates brand/dist from brand/tokens.json and the vendored
// font. Run it through `mise run gen`; check-generated (and CI) catch a
// dist/ whose text files are out of date, and brand/internal/assets' tests
// catch stale images.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/roguekeyboards/kbdb/brand/internal/assets"
)

func main() {
	dir := flag.String("dir", "brand", "brand directory: reads tokens.json and fonts/, writes dist/")
	flag.Parse()
	if err := run(*dir); err != nil {
		log.Fatal(err)
	}
}

func run(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	tokensJSON, err := root.ReadFile(assets.TokensFile)
	if err != nil {
		return err
	}
	font, err := root.ReadFile(assets.FontFile)
	if err != nil {
		return err
	}
	files, err := assets.Build(tokensJSON, font)
	if err != nil {
		return err
	}

	if err := root.RemoveAll(assets.DistDir); err != nil {
		return err
	}
	if err := root.MkdirAll(assets.DistDir, 0o750); err != nil {
		return err
	}
	for name, data := range files {
		if err := root.WriteFile(assets.DistDir+"/"+name, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
