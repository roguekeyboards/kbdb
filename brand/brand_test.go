package brand

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BrandSuite struct {
	suite.Suite
}

func TestBrandSuite(t *testing.T) {
	suite.Run(t, new(BrandSuite))
}

func (s *BrandSuite) TestFilesHoldsTheGeneratedAssets() {
	for _, name := range []string{
		"favicon.svg", "favicon.ico", "favicon-16.png", "favicon-32.png", "apple-touch-icon.png",
		"icon-192.png", "icon-512.png", "mark-on-dark.svg", "mark-on-light.svg",
		"logo-horizontal-on-dark.svg", "logo-horizontal-on-light.svg",
		"logo-stacked-on-dark.svg", "logo-stacked-on-light.svg",
		"discord-avatar.png", "discord-avatar-dev.png", "tokens.css",
	} {
		_, err := fs.Stat(Files, name)
		s.NoError(err, name)
	}
}

func (s *BrandSuite) TestTokensParse() {
	accent, ok := Tokens.Theme.Lookup("accent")
	s.True(ok)
	s.Equal("#e8a33d", accent)
}

// The logo is drawn in the theme's own colours, so the two sections must
// agree wherever they describe the same colour.
func (s *BrandSuite) TestLogoColoursMatchTheTheme() {
	for logo, theme := range map[string]string{"ink": "bg", "cream": "text", "amber": "accent"} {
		l, ok := Tokens.Logo.Lookup(logo)
		s.Require().True(ok, logo)
		t, ok := Tokens.Theme.Lookup(theme)
		s.Require().True(ok, theme)
		s.Equal(t, l, "logo %q should equal theme %q", logo, theme)
	}
}
