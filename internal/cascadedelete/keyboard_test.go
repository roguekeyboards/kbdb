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

type DeleteKeyboardSuite struct {
	suite.Suite

	mockKeyboards      *mocks.MockKeyboardRepository
	mockBuilds         *mocks.MockBuildRepository
	mockBuildImages    *mocks.MockBuildImageStore
	mockKeyboardImages *mocks.MockKeyboardImageStore
	ctx                context.Context
}

func TestDeleteKeyboardSuite(t *testing.T) {
	suite.Run(t, new(DeleteKeyboardSuite))
}

func (s *DeleteKeyboardSuite) SetupTest() {
	s.mockKeyboards = mocks.NewMockKeyboardRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.mockKeyboardImages = mocks.NewMockKeyboardImageStore(s.T())
	s.ctx = s.T().Context()
}

func (s *DeleteKeyboardSuite) TestBlock_NoReferencingBuilds_DeletesKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return(nil, nil)
	s.mockKeyboards.EXPECT().
		Get(s.ctx, "alice", "kb1").
		Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeyboards.EXPECT().
		Delete(s.ctx, "kb1").
		Return(nil)

	result, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeyboardSuite) TestBlock_KeyboardHadImages_DeletesEachFromS3BeforeDB() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return(nil, nil)
	s.mockKeyboards.EXPECT().
		Get(s.ctx, "alice", "kb1").
		Return(&repository.Keyboard{ID: "kb1", Images: repository.KeyboardImagesMap([]repository.KeyboardImage{
			{ImageID: "img1", Path: "keyboards/alice/kb1/images/img1"},
		})}, nil)
	s.mockKeyboardImages.EXPECT().
		DeleteKeyboardImage(s.ctx, repository.KeyboardImageKey("keyboards/alice/kb1/images/img1")).
		Return(nil)
	s.mockKeyboards.EXPECT().
		Delete(s.ctx, "kb1").
		Return(nil)

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
}

func (s *DeleteKeyboardSuite) TestBlock_ImageDeleteFails_ReturnsErrorWithoutDeletingKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return(nil, nil)
	s.mockKeyboards.EXPECT().
		Get(s.ctx, "alice", "kb1").
		Return(&repository.Keyboard{ID: "kb1", Images: repository.KeyboardImagesMap([]repository.KeyboardImage{
			{ImageID: "img1", Path: "keyboards/alice/kb1/images/img1"},
		})}, nil)
	s.mockKeyboardImages.EXPECT().
		DeleteKeyboardImage(s.ctx, repository.KeyboardImageKey("keyboards/alice/kb1/images/img1")).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	// mockKeyboards has no .EXPECT() for Delete - verifies the DB record
	// was never touched, so a retry can safely re-attempt the S3 delete.
}

func (s *DeleteKeyboardSuite) TestBlock_ReferencingBuilds_ReturnsBlockedError_DeletesNothing() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return([]string{"build-1", "build-2"}, nil)

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, cascadedelete.ErrBlocked)

	var blocked *cascadedelete.BlockedError
	s.Require().ErrorAs(err, &blocked)
	s.ElementsMatch([]string{"build-1", "build-2"}, blocked.BuildIDs)

	// mockKeyboards has no .EXPECT() calls set up - mockery's mock.Mock
	// fails the test if Get/Delete is called unexpectedly, verifying
	// nothing was deleted.
}

func (s *DeleteKeyboardSuite) TestCascade_NoReferencingBuilds_DeletesOnlyKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return(nil, nil)
	s.mockKeyboards.EXPECT().
		Get(s.ctx, "alice", "kb1").
		Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeyboards.EXPECT().
		Delete(s.ctx, "kb1").
		Return(nil)

	result, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeyboardSuite) TestCascade_ReferencingBuilds_DeletesEachBuildImagesThenBuildThenKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
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
	s.mockKeyboards.EXPECT().
		Get(s.ctx, "alice", "kb1").
		Return(&repository.Keyboard{ID: "kb1"}, nil)
	s.mockKeyboards.EXPECT().
		Delete(s.ctx, "kb1").
		Return(nil)

	result, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.ElementsMatch([]string{"build-1", "build-2"}, result.DeletedBuildIDs)
}

func (s *DeleteKeyboardSuite) TestCascade_BuildImageDeleteFails_ReturnsErrorWithoutDeletingBuildOrKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1", Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: "builds/alice/build-1/images/img1"},
		})}, nil)
	s.mockBuildImages.EXPECT().
		DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/build-1/images/img1")).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockBuilds has no .EXPECT() for Delete, mockKeyboards has none at
	// all - verifies neither the build nor the keyboard was touched.
}

func (s *DeleteKeyboardSuite) TestCascade_BuildDeleteFails_ReturnsErrorWithoutDeletingKeyboard() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1"}, nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-1").
		Return(errors.New("dynamo unavailable"))

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockKeyboards has no .EXPECT() - verifies Delete was never reached.
}

func (s *DeleteKeyboardSuite) TestUnknownOnDelete_ReturnsError_CallsNothing() {
	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDelete("bogus"),
	)

	s.Require().Error(err)
	// mockKeyboards and mockBuilds have no .EXPECT() calls set up -
	// verifies the unknown value is rejected before any repository call.
}

func (s *DeleteKeyboardSuite) TestFindReferencingBuildsFails_ReturnsError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").
		Return(nil, errors.New("query failed"))

	_, err := cascadedelete.DeleteKeyboard(
		s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, s.mockKeyboardImages,
		"alice", "kb1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	s.Require().NotErrorIs(err, cascadedelete.ErrBlocked)
}
