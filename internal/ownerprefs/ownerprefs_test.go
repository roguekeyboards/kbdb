package ownerprefs

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

type OwnerPrefsSuite struct {
	suite.Suite
}

func TestOwnerPrefsSuite(t *testing.T) {
	suite.Run(t, new(OwnerPrefsSuite))
}

func (s *OwnerPrefsSuite) TestGet_NoPreferences_ReturnsErrMissing() {
	_, err := Get(s.T().Context())

	s.Require().ErrorIs(err, ErrMissing)
}

func (s *OwnerPrefsSuite) TestGet_ReturnsStoredPreferences() {
	want := repository.ProfilePreferences{Currency: "EUR", ShowPriceToOthers: true}

	got, err := Get(WithPreferences(s.T().Context(), want))

	s.Require().NoError(err)
	s.Equal(want, got)
}
