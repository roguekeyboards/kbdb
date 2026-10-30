package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

type NewBuildImageKeySuite struct {
	suite.Suite
}

func TestNewBuildImageKeySuite(t *testing.T) {
	suite.Run(t, new(NewBuildImageKeySuite))
}

func (s *NewBuildImageKeySuite) TestSucceeds() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "alice")

	key, err := repository.NewBuildImageKey(ctx, "b1", "img1")

	s.Require().NoError(err)
	s.Equal(repository.BuildImageKey("builds/alice/b1/images/img1"), key)
}

func (s *NewBuildImageKeySuite) TestNoUserIDInContext_ReturnsError() {
	key, err := repository.NewBuildImageKey(context.Background(), "b1", "img1")

	s.Require().ErrorIs(err, repository.ErrNoUserID)
	s.Empty(key)
}
