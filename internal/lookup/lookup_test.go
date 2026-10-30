package lookup_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/lookup"
)

type ValidateImageContentTypeSuite struct {
	suite.Suite
}

func TestValidateImageContentTypeSuite(t *testing.T) {
	suite.Run(t, new(ValidateImageContentTypeSuite))
}

func (s *ValidateImageContentTypeSuite) TestValid_ReturnsNoError() {
	errs := lookup.ValidateImageContentType(s.T().Context(), "image/webp")
	s.Empty(errs)
}

func (s *ValidateImageContentTypeSuite) TestInvalid_ReturnsFieldError() {
	errs := lookup.ValidateImageContentType(s.T().Context(), "image/png")
	s.Equal([]lookup.FieldError{
		{Field: "content_type", Value: "image/png", Reason: `"image/png" is not an approved image_content_type value`},
	}, errs)
}
