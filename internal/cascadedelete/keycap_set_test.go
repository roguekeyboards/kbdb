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

type DeleteKeycapSetSuite struct {
	suite.Suite

	mockKeycapSets  *mocks.MockKeycapSetRepository
	mockBuilds      *mocks.MockBuildRepository
	mockBuildImages *mocks.MockBuildImageStore
	mockKitImages   *mocks.MockKeycapKitImageStore
	ctx             context.Context
}

func TestDeleteKeycapSetSuite(t *testing.T) {
	suite.Run(t, new(DeleteKeycapSetSuite))
}

func (s *DeleteKeycapSetSuite) SetupTest() {
	s.mockKeycapSets = mocks.NewMockKeycapSetRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.mockKitImages = mocks.NewMockKeycapKitImageStore(s.T())
	s.ctx = s.T().Context()
}

func (s *DeleteKeycapSetSuite) TestBlock_NoReferencingBuilds_DeletesSet() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1"}, nil)
	s.mockKeycapSets.EXPECT().
		Delete(s.ctx, "ks1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeycapSetSuite) TestBlock_SetHadKitImages_DeletesEachFromS3BeforeDB() {
	key1 := repository.KeycapKitImageKey("keycap-sets/alice/ks1/kits/kit1/image/up1")
	key2 := repository.KeycapKitImageKey("keycap-sets/alice/ks1/kits/kit2/image/up1")
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1", ImagePath: &key1},
			"kit2": {KitID: "kit2", ImagePath: &key2},
			"kit3": {KitID: "kit3"}}}, nil)
	s.mockKitImages.EXPECT().
		Delete(s.ctx, key1).
		Return(nil)
	s.mockKitImages.EXPECT().
		Delete(s.ctx, key2).
		Return(nil)
	s.mockKeycapSets.EXPECT().
		Delete(s.ctx, "ks1").
		Return(nil)

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
}

func (s *DeleteKeycapSetSuite) TestBlock_KitImageDeleteFails_ReturnsErrorWithoutDeletingSet() {
	key1 := repository.KeycapKitImageKey("keycap-sets/alice/ks1/kits/kit1/image/up1")
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1", ImagePath: &key1}}}, nil)
	s.mockKitImages.EXPECT().
		Delete(s.ctx, key1).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	// mockKeycapSets has no .EXPECT() for Delete - verifies the DB record
	// was never touched, so a retry can safely re-attempt the S3 delete.
}

func (s *DeleteKeycapSetSuite) TestBlock_ReferencingBuilds_ReturnsBlockedError_DeletesNothing() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return([]string{"build-1", "build-2"}, nil)

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, cascadedelete.ErrBlocked)

	var blocked *cascadedelete.BlockedError
	s.Require().ErrorAs(err, &blocked)
	s.ElementsMatch([]string{"build-1", "build-2"}, blocked.BuildIDs)

	// mockKeycapSets has no .EXPECT() calls set up - mockery's mock.Mock
	// fails the test if Get/Delete is called unexpectedly, verifying
	// nothing was deleted.
}

func (s *DeleteKeycapSetSuite) TestCascade_NoReferencingBuilds_DeletesOnlySet() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1"}, nil)
	s.mockKeycapSets.EXPECT().
		Delete(s.ctx, "ks1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeycapSetSuite) TestCascade_ReferencingBuilds_DeletesEachBuildImagesThenBuildThenSet() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
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
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1"}, nil)
	s.mockKeycapSets.EXPECT().
		Delete(s.ctx, "ks1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.ElementsMatch([]string{"build-1", "build-2"}, result.DeletedBuildIDs)
}

func (s *DeleteKeycapSetSuite) TestCascade_BuildImageDeleteFails_ReturnsErrorWithoutDeletingBuildOrSet() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1", Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: "builds/alice/build-1/images/img1"},
		})}, nil)
	s.mockBuildImages.EXPECT().
		DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/build-1/images/img1")).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockBuilds has no .EXPECT() for Delete, mockKeycapSets has none at
	// all - verifies neither the build nor the set was touched.
}

func (s *DeleteKeycapSetSuite) TestCascade_BuildDeleteFails_ReturnsErrorWithoutDeletingSet() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1"}, nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-1").
		Return(errors.New("dynamo unavailable"))

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockKeycapSets has no .EXPECT() - verifies Delete was never reached.
}

func (s *DeleteKeycapSetSuite) TestUnknownOnDelete_ReturnsError_CallsNothing() {
	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDelete("bogus"),
	)

	s.Require().Error(err)
	// mockKeycapSets and mockBuilds have no .EXPECT() calls set up -
	// verifies the unknown value is rejected before any repository call.
}

func (s *DeleteKeycapSetSuite) TestFindReferencingBuildsFails_ReturnsError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, errors.New("query failed"))

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	s.Require().NotErrorIs(err, cascadedelete.ErrBlocked)
}

func (s *DeleteKeycapSetSuite) TestBlock_GetSetNotFound_SucceedsIdempotently() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapSet(s.ctx, "alice", "ks1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(nil, repository.ErrNotFound)

	_, err := cascadedelete.DeleteKeycapSet(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err, "a not-found set makes the delete idempotently succeed")
	// mockKeycapSets has no .EXPECT() for Delete - verifies it was never
	// reached.
}
