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

type DeleteKeycapKitSuite struct {
	suite.Suite

	mockKeycapSets  *mocks.MockKeycapSetRepository
	mockBuilds      *mocks.MockBuildRepository
	mockBuildImages *mocks.MockBuildImageStore
	mockKitImages   *mocks.MockKeycapKitImageStore
	ctx             context.Context
}

func TestDeleteKeycapKitSuite(t *testing.T) {
	suite.Run(t, new(DeleteKeycapKitSuite))
}

func (s *DeleteKeycapKitSuite) SetupTest() {
	s.mockKeycapSets = mocks.NewMockKeycapSetRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.mockKitImages = mocks.NewMockKeycapKitImageStore(s.T())
	s.ctx = s.T().Context()
}

func (s *DeleteKeycapKitSuite) TestBlock_NoReferencingBuilds_DeletesKit() {
	imgKey := repository.KeycapKitImageKey("img-1")
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1", ImagePath: &imgKey}}}, nil)
	s.mockKitImages.EXPECT().
		Delete(s.ctx, imgKey).
		Return(nil)
	s.mockKeycapSets.EXPECT().
		DeleteKit(s.ctx, "ks1", "kit1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeycapKitSuite) TestBlock_ImageDeleteFails_ReturnsErrorWithoutDeletingKit() {
	imgKey := repository.KeycapKitImageKey("img-1")
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1", ImagePath: &imgKey}}}, nil)
	s.mockKitImages.EXPECT().
		Delete(s.ctx, imgKey).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	// mockKeycapSets has no .EXPECT() for DeleteKit - verifies the DB
	// record was never touched, so a retry can safely re-attempt the S3
	// delete.
}

func (s *DeleteKeycapKitSuite) TestBlock_ReferencingBuilds_ReturnsBlockedError_DeletesNothing() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return([]string{"build-1", "build-2"}, nil)

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, cascadedelete.ErrBlocked)

	var blocked *cascadedelete.BlockedError
	s.Require().ErrorAs(err, &blocked)
	s.ElementsMatch([]string{"build-1", "build-2"}, blocked.BuildIDs)

	// mockKeycapSets has no .EXPECT() calls set up - mockery's mock.Mock
	// fails the test if Get/DeleteKit is called unexpectedly, verifying
	// nothing was deleted.
}

func (s *DeleteKeycapKitSuite) TestCascade_NoReferencingBuilds_DeletesOnlyKit() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1"}}}, nil)
	s.mockKeycapSets.EXPECT().
		DeleteKit(s.ctx, "ks1", "kit1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeycapKitSuite) TestCascade_ReferencingBuilds_DeletesEachBuildImagesThenBuildThenKit() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
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
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1"}}}, nil)
	s.mockKeycapSets.EXPECT().
		DeleteKit(s.ctx, "ks1", "kit1").
		Return(nil)

	result, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteCascade,
	)

	s.Require().NoError(err)
	s.ElementsMatch([]string{"build-1", "build-2"}, result.DeletedBuildIDs)
}

func (s *DeleteKeycapKitSuite) TestCascade_BuildImageDeleteFails_ReturnsErrorWithoutDeletingBuildOrKit() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1", Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: "builds/alice/build-1/images/img1"},
		})}, nil)
	s.mockBuildImages.EXPECT().
		DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/build-1/images/img1")).
		Return(errors.New("s3 unavailable"))

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockBuilds has no .EXPECT() for Delete, mockKeycapSets has none at
	// all - verifies neither the build nor the kit was touched.
}

func (s *DeleteKeycapKitSuite) TestCascade_BuildDeleteFails_ReturnsErrorWithoutDeletingKit() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return([]string{"build-1"}, nil)
	s.mockBuilds.EXPECT().
		Get(s.ctx, "alice", "build-1").
		Return(&repository.Build{ID: "build-1"}, nil)
	s.mockBuilds.EXPECT().
		Delete(s.ctx, "build-1").
		Return(errors.New("dynamo unavailable"))

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteCascade,
	)

	s.Require().Error(err)
	// mockKeycapSets has no .EXPECT() - verifies DeleteKit was never
	// reached.
}

func (s *DeleteKeycapKitSuite) TestUnknownOnDelete_ReturnsError_CallsNothing() {
	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDelete("bogus"),
	)

	s.Require().Error(err)
	// mockKeycapSets and mockBuilds have no .EXPECT() calls set up -
	// verifies the unknown value is rejected before any repository call.
}

func (s *DeleteKeycapKitSuite) TestFindReferencingBuildsFails_ReturnsError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, errors.New("query failed"))

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().Error(err)
	s.Require().NotErrorIs(err, cascadedelete.ErrBlocked)
}

func (s *DeleteKeycapKitSuite) TestBlock_ParentSetNotFound_PropagatesError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(nil, repository.ErrNotFound)

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, repository.ErrNotFound, "unlike a missing kit within an existing set, a missing parent set is a real not-found")
}

func (s *DeleteKeycapKitSuite) TestBlock_KitNotInSet_SucceedsIdempotently() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1"}, nil)

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().NoError(err)
	// mockKeycapSets has no .EXPECT() for DeleteKit - a kit not present in
	// the set is idempotent, matching DeleteKit's own idempotency.
}

func (s *DeleteKeycapKitSuite) TestBlock_DeleteKitMutationConflict_PropagatesError() {
	s.mockBuilds.EXPECT().
		FindBuildsReferencingKeycapKit(s.ctx, "alice", "ks1", "kit1").
		Return(nil, nil)
	s.mockKeycapSets.EXPECT().
		Get(s.ctx, "alice", "ks1").
		Return(&repository.KeycapSet{ID: "ks1", Kits: map[string]repository.KeycapKit{"kit1": {KitID: "kit1"}}}, nil)
	s.mockKeycapSets.EXPECT().
		DeleteKit(s.ctx, "ks1", "kit1").
		Return(repository.ErrMutationConflict)

	_, err := cascadedelete.DeleteKeycapKit(
		s.ctx, s.mockKeycapSets, s.mockBuilds, s.mockBuildImages, s.mockKitImages,
		"alice", "ks1", "kit1", cascadedelete.OnDeleteBlock,
	)

	s.Require().ErrorIs(err, repository.ErrMutationConflict)
}
