package cascadedelete_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/cascadedelete"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type DeleteBuildSuite struct {
	suite.Suite

	mockBuilds      *mocks.MockBuildRepository
	mockBuildImages *mocks.MockBuildImageStore
	ctx             context.Context
}

func TestDeleteBuildSuite(t *testing.T) {
	suite.Run(t, new(DeleteBuildSuite))
}

func (s *DeleteBuildSuite) SetupTest() {
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.ctx = s.T().Context()
}

func (s *DeleteBuildSuite) delete() error {
	return cascadedelete.DeleteBuild(s.ctx, s.mockBuilds, s.mockBuildImages, "alice", "b1")
}

func (s *DeleteBuildSuite) TestDeletesImagesBeforeRecord() {
	key := repository.BuildImageKey("builds/alice/b1/img1")
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b1").
		Return(&repository.Build{ID: "b1", Images: map[string]repository.BuildImageEntry{"img1": {Path: key}}}, nil)
	var order []string
	s.mockBuildImages.EXPECT().DeleteBuildImage(s.ctx, key).
		Run(func(context.Context, repository.BuildImageKey) { order = append(order, "s3") }).
		Return(nil)
	s.mockBuilds.EXPECT().Delete(s.ctx, "b1").
		Run(func(context.Context, string) { order = append(order, "db") }).
		Return(nil)

	s.Require().NoError(s.delete())
	s.Equal([]string{"s3", "db"}, order)
}

func (s *DeleteBuildSuite) TestAlreadyGone_NoError() {
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b1").Return(nil, repository.ErrNotFound)

	s.Require().NoError(s.delete())
}

func (s *DeleteBuildSuite) TestImageDeleteFails_RecordKept() {
	key := repository.BuildImageKey("builds/alice/b1/img1")
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b1").
		Return(&repository.Build{ID: "b1", Images: map[string]repository.BuildImageEntry{"img1": {Path: key}}}, nil)
	s.mockBuildImages.EXPECT().DeleteBuildImage(s.ctx, key).Return(errors.New("s3 down"))

	s.Require().Error(s.delete())
	s.mockBuilds.AssertNotCalled(s.T(), "Delete", mock.Anything, mock.Anything)
}
