package repository_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

type VisibilitySuite struct {
	suite.Suite
}

func TestVisibilitySuite(t *testing.T) {
	suite.Run(t, new(VisibilitySuite))
}

func (s *VisibilitySuite) TestValid_Public_ReturnsTrue() {
	s.True(repository.VisibilityPublic.Valid())
}

func (s *VisibilitySuite) TestValid_Authenticated_ReturnsTrue() {
	s.True(repository.VisibilityAuthenticated.Valid())
}

func (s *VisibilitySuite) TestValid_Private_ReturnsTrue() {
	s.True(repository.VisibilityPrivate.Valid())
}

func (s *VisibilitySuite) TestValid_Empty_ReturnsFalse() {
	s.False(repository.Visibility("").Valid())
}

func (s *VisibilitySuite) TestValid_Unknown_ReturnsFalse() {
	s.False(repository.Visibility("shared").Valid())
}
