package repoapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/lookup"
)

type LookupToAPISuite struct {
	suite.Suite
}

func TestLookupToAPISuite(t *testing.T) {
	suite.Run(t, new(LookupToAPISuite))
}

// mustGetCategory looks up a real, catalog-sourced Lookup - LookupToAPI
// requires one, not a hand-built Lookup.
func (s *LookupToAPISuite) mustGetCategory(category lookup.Category) lookup.Lookup {
	l, ok := lookup.GetCategory(context.Background(), category)
	s.Require().True(ok, "category %q not found", category)
	return l
}

func (s *LookupToAPISuite) TestPlainStringCategory_PassesThrough() {
	l := s.mustGetCategory(lookup.CategoryVendor)

	got := Lookup{}.ToAPI(l)

	s.Equal("vendor", got.Category)
	s.Contains(got.Values, "Amazon")
}

func (s *LookupToAPISuite) TestKeyboardLayout_DecodesTyped() {
	l := s.mustGetCategory(lookup.CategoryKeyboardLayout)

	got := Lookup{}.ToAPI(l)

	s.NotEmpty(got.Values)
	s.IsType(lookup.LayoutValue{}, got.Values[0])
}

func (s *LookupToAPISuite) TestBuildCaseMountType_DecodesTyped() {
	l := s.mustGetCategory(lookup.CategoryBuildCaseMountType)

	got := Lookup{}.ToAPI(l)

	s.NotEmpty(got.Values)
	s.IsType(lookup.CaseMountTypeValue{}, got.Values[0])
}
