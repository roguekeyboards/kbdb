package cascadedelete_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/cascadedelete"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

type DeleteKeyboardPartSuite struct {
	suite.Suite

	mockKeyboards   *mocks.MockKeyboardRepository
	mockBuilds      *mocks.MockBuildRepository
	mockBuildImages *mocks.MockBuildImageStore
	ctx             context.Context
}

func TestDeleteKeyboardPartSuite(t *testing.T) {
	suite.Run(t, new(DeleteKeyboardPartSuite))
}

func (s *DeleteKeyboardPartSuite) SetupTest() {
	s.mockKeyboards = mocks.NewMockKeyboardRepository(s.T())
	s.mockBuilds = mocks.NewMockBuildRepository(s.T())
	s.mockBuildImages = mocks.NewMockBuildImageStore(s.T())
	s.ctx = s.T().Context()
}

// keyboardBuilds stubs kb1's builds: b1 uses plate p1 and PCB b1, b2 uses
// plate p2, and b3 was deleted after its marker was listed.
func (s *DeleteKeyboardPartSuite) keyboardBuilds() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").Return([]string{"b1", "b2", "b3"}, nil)
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b1").
		Return(&repository.Build{ID: "b1", Plate: new("p1"), PCB: new("pcb1"), Images: repository.BuildImagesMap([]repository.BuildImage{
			{ImageID: "img1", Path: repository.BuildImageKey("builds/alice/b1/images/img1")},
		})}, nil)
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b2").Return(&repository.Build{ID: "b2", Plate: new("p2")}, nil)
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b3").Return(nil, repository.ErrNotFound)
}

func (s *DeleteKeyboardPartSuite) TestPlate_NoBuildUsesIt_DeletesPlate() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").Return([]string{"b2"}, nil)
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b2").Return(&repository.Build{ID: "b2", Plate: new("p2")}, nil)
	s.mockKeyboards.EXPECT().DeletePlate(s.ctx, "kb1", "p1").Return(nil)

	result, err := cascadedelete.DeleteKeyboardPlate(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "p1", cascadedelete.OnDeleteBlock)

	s.Require().NoError(err)
	s.Empty(result.DeletedBuildIDs)
}

func (s *DeleteKeyboardPartSuite) TestPlate_Block_ListsOnlyBuildsUsingIt() {
	s.keyboardBuilds()

	_, err := cascadedelete.DeleteKeyboardPlate(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "p1", cascadedelete.OnDeleteBlock)

	var blocked *cascadedelete.BlockedError
	s.Require().ErrorAs(err, &blocked)
	s.Equal([]string{"b1"}, blocked.BuildIDs)
}

func (s *DeleteKeyboardPartSuite) TestPCB_Cascade_DeletesUsingBuildsWithImagesThenPCB() {
	s.keyboardBuilds()
	s.mockBuildImages.EXPECT().DeleteBuildImage(s.ctx, repository.BuildImageKey("builds/alice/b1/images/img1")).Return(nil)
	s.mockBuilds.EXPECT().Delete(s.ctx, "b1").Return(nil)
	s.mockKeyboards.EXPECT().DeletePCB(s.ctx, "kb1", "pcb1").Return(nil)

	result, err := cascadedelete.DeleteKeyboardPCB(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "pcb1", cascadedelete.OnDeleteCascade)

	s.Require().NoError(err)
	s.Equal([]string{"b1"}, result.DeletedBuildIDs)
}

func (s *DeleteKeyboardPartSuite) TestCascade_BuildDeleteFails_KeepsPart() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").Return([]string{"b2"}, nil)
	s.mockBuilds.EXPECT().Get(s.ctx, "alice", "b2").Return(&repository.Build{ID: "b2", Plate: new("p2")}, nil)
	s.mockBuilds.EXPECT().Delete(s.ctx, "b2").Return(errors.New("throttled"))

	_, err := cascadedelete.DeleteKeyboardPlate(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "p2", cascadedelete.OnDeleteCascade)

	s.Require().Error(err)
}

func (s *DeleteKeyboardPartSuite) TestUnknownOnDelete_ReturnsErrorWithoutLookingUpBuilds() {
	_, err := cascadedelete.DeleteKeyboardPlate(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "p1", cascadedelete.OnDelete("detach"))

	s.Require().Error(err)
}

func (s *DeleteKeyboardPartSuite) TestFindBuildsError_Propagates() {
	s.mockBuilds.EXPECT().FindBuildsReferencingKeyboard(s.ctx, "alice", "kb1").Return(nil, errors.New("throttled"))

	_, err := cascadedelete.DeleteKeyboardPCB(s.ctx, s.mockKeyboards, s.mockBuilds, s.mockBuildImages, "alice", "kb1", "pcb1", cascadedelete.OnDeleteBlock)

	s.Require().Error(err)
}
