package cascadedelete_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/mykeebs-api/internal/cascadedelete"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
	"github.com/roguekeyboards/mykeebs-api/internal/repository/mocks"
)

type DeleteSwitchSuite struct {
	suite.Suite

	mockSwitches     *mocks.MockSwitchRepository
	mockBuilds       *mocks.MockBuildRepository
	mockBuildImages  *mocks.MockBuildImageStore
	mockSwitchImages *mocks.MockSwitchImageStore
	ctx              context.Context
}

func TestDeleteSwitchSuite(t *testing.T) {
	suite.Run(t, new(DeleteSwitchSuite))
}

func (s *DeleteSwitchSuite) SetupTest() {
	s.mockSwitches = mocks.NewMockSwitchRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.mockSwitchImages = mocks.NewMockSwitchImageStore(s.T())
	s.ctx = s.T().Context()
}

func (s *DeleteSwitchSuite) TestBlock_NoReferencingBuilds_DeletesSwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return(nil, nil)
	s.mockSwitches.EXPECT().
		Get(s.ctx, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1"}, nil)
	s.mockSwitches.EXPECT().
		Delete(s.ctx, "sw1").
		Return(nil)

	result, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteSwitchSuite) TestBlock_SwitchHadImage_DeletesFromS3BeforeDB() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return(nil, nil)
	key := repository.SwitchImageKey("switches/alice/sw1/image/up1")
	s.mockSwitches.EXPECT().
		Get(s.ctx, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1", ImagePath: &key}, nil)
	s.mockSwitchImages.EXPECT().
		Delete(s.ctx, key).
		Return(nil)
	s.mockSwitches.EXPECT().
		Delete(s.ctx, "sw1").
		Return(nil)

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
}

func (s *DeleteSwitchSuite) TestBlock_ImageDeleteFails_ReturnsErrorWithoutDeletingSwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return(nil, nil)
	key := repository.SwitchImageKey("switches/alice/sw1/image/up1")
	s.mockSwitches.EXPECT().
		Get(s.ctx, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1", ImagePath: &key}, nil)
	s.mockSwitchImages.EXPECT().
		Delete(s.ctx, key).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	// mockSwitches has no .EXPECT() for Delete - verifies the DB record
	// was never touched, so a retry can safely re-attempt the S3 delete.
}

func (s *DeleteSwitchSuite) TestBlock_ReferencingBuilds_ReturnsBlockedError_DeletesNothing() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return([]string{"build-1", "build-2"}, nil)

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, cascadedelete.ErrBlocked)

	var blocked *cascadedelete.BlockedError
	s.Require().ErrorAs(err, &blocked)
	s.ElementsMatch([]string{"build-1", "build-2"}, blocked.BuildIDs)

	// mockSwitches has no .EXPECT() calls set up - mockery's mock.Mock
	// fails the test if Get/Delete is called unexpectedly, verifying
	// nothing was deleted.
}

func (s *DeleteSwitchSuite) TestCascade_NoReferencingBuilds_DeletesOnlySwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return(nil, nil)
	s.mockSwitches.EXPECT().
		Get(s.ctx, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1"}, nil)
	s.mockSwitches.EXPECT().
		Delete(s.ctx, "sw1").
		Return(nil)

	result, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteSwitchSuite) TestCascade_ReferencingBuilds_DeletesEachBuildImagesThenBuildThenSwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return([]string{"build-1", "build-2"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1", Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: "builds/alice/build-1/images/img1"},
		})}, nil)
	s.mockBuildImages.EXPECT().
		DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/build-1/images/img1")).
		Return(nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-1").
		Return(nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-2").
		Return(&repository.Build{ID: "build-2"}, nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-2").
		Return(nil)
	s.mockSwitches.EXPECT().
		Get(s.ctx, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1"}, nil)
	s.mockSwitches.EXPECT().
		Delete(s.ctx, "sw1").
		Return(nil)

	result, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.ElementsMatch([]string{"build-1", "build-2"}, result.DeletedBuildIDs)
}

func (s *DeleteSwitchSuite) TestCascade_BuildImageDeleteFails_ReturnsErrorWithoutDeletingBuildOrSwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1", Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: "builds/alice/build-1/images/img1"},
		})}, nil)
	s.mockBuildImages.EXPECT().
		DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/build-1/images/img1")).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockBuilds has no .EXPECT() for Delete, mockSwitches has none at
	// all - verifies neither the build nor the switch was touched.
}

func (s *DeleteSwitchSuite) TestCascade_BuildDeleteFails_ReturnsErrorWithoutDeletingSwitch() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1"}, nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-1").
		Return(errors.New("dynamo unavailable"))

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockSwitches has no .EXPECT() - verifies Delete was never reached.
}

func (s *DeleteSwitchSuite) TestUnknownOnDelete_ReturnsError_CallsNothing() {
	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDelete("bogus"),
	)

	s.Require().Error(err)
	// mockSwitches and mockBuilds have no .EXPECT() calls set up - verifies
	// the unknown value is rejected before any repository call.
}

func (s *DeleteSwitchSuite) TestFindReferencingBuildsFails_ReturnsError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingSwitch(s.ctx, "alice", "sw1").
		Return(nil, errors.New("query failed"))

	_, err := cascadedelete.DeleteSwitch(
		s.ctx, s.mockSwitches, s.mockBuilds, s.mockBuildImages, s.mockSwitchImages,
		"alice", "sw1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	s.Require().NotErrorIs(err, cascadedelete.ErrBlocked)
}
