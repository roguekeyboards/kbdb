package accountdelete_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/accountdelete"
	"github.com/rogueserenity/kbdb/internal/repository"
	"github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type DeleteSuite struct {
	suite.Suite

	profiles      *mocks.MockProfileRepository
	profileImages *mocks.MockProfileImageStore
	builds        *mocks.MockBuildRepository
	buildImages   *mocks.MockBuildImageStore
	keyboards     *mocks.MockKeyboardRepository
	switches      *mocks.MockSwitchRepository
	sets          *mocks.MockKeycapSetRepository
	account       accountdelete.Account

	mu    sync.Mutex
	order []string
}

func TestDeleteSuite(t *testing.T) {
	suite.Run(t, new(DeleteSuite))
}

func (s *DeleteSuite) SetupTest() {
	s.profiles = mocks.NewMockProfileRepository(s.T())
	s.profileImages = mocks.NewMockProfileImageStore(s.T())
	s.builds = mocks.NewMockBuildRepository(s.T())
	s.buildImages = mocks.NewMockBuildImageStore(s.T())
	s.keyboards = mocks.NewMockKeyboardRepository(s.T())
	s.switches = mocks.NewMockSwitchRepository(s.T())
	s.sets = mocks.NewMockKeycapSetRepository(s.T())
	s.account = accountdelete.Account{
		Profiles:       s.profiles,
		ProfileImages:  s.profileImages,
		Builds:         s.builds,
		BuildImages:    s.buildImages,
		Keyboards:      s.keyboards,
		KeyboardImages: mocks.NewMockKeyboardImageStore(s.T()),
		Switches:       s.switches,
		SwitchImages:   mocks.NewMockSwitchImageStore(s.T()),
		KeycapSets:     s.sets,
		KitImages:      mocks.NewMockKeycapKitImageStore(s.T()),
	}
	s.order = nil
}

func (s *DeleteSuite) record(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = append(s.order, step)
}

// expectItems sets up one build, keyboard, switch and keycap set, each
// deleted with no build still referencing it.
func (s *DeleteSuite) expectItems() {
	s.builds.EXPECT().List(mock.Anything, "alice", mock.Anything, "", mock.Anything, "").
		Return([]repository.Build{{ID: "b1"}}, "", nil)
	s.builds.EXPECT().Get(mock.Anything, "alice", "b1").Return(&repository.Build{ID: "b1"}, nil)
	s.builds.EXPECT().Delete(mock.Anything, "b1").
		Run(func(context.Context, string) { s.record("build") }).Return(nil)

	s.keyboards.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").
		Return([]repository.Keyboard{{ID: "k1"}}, "", nil)
	s.builds.EXPECT().FindBuildsReferencingKeyboard(mock.Anything, "alice", "k1").Return(nil, nil)
	s.keyboards.EXPECT().Get(mock.Anything, "alice", "k1").Return(&repository.Keyboard{ID: "k1"}, nil)
	s.keyboards.EXPECT().Delete(mock.Anything, "k1").
		Run(func(context.Context, string) { s.record("keyboard") }).Return(nil)

	s.switches.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").
		Return([]repository.Switch{{ID: "s1"}}, "", nil)
	s.builds.EXPECT().FindBuildsReferencingSwitch(mock.Anything, "alice", "s1").Return(nil, nil)
	s.switches.EXPECT().Get(mock.Anything, "alice", "s1").Return(&repository.Switch{ID: "s1"}, nil)
	s.switches.EXPECT().Delete(mock.Anything, "s1").
		Run(func(context.Context, string) { s.record("switch") }).Return(nil)

	s.sets.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").
		Return([]repository.KeycapSet{{ID: "ks1"}}, "", nil)
	s.builds.EXPECT().FindBuildsReferencingKeycapSet(mock.Anything, "alice", "ks1").Return(nil, nil)
	s.sets.EXPECT().Get(mock.Anything, "alice", "ks1").Return(&repository.KeycapSet{ID: "ks1"}, nil)
	s.sets.EXPECT().Delete(mock.Anything, "ks1").
		Run(func(context.Context, string) { s.record("keycap set") }).Return(nil)
}

func (s *DeleteSuite) TestDeletesBuildsFirstAndProfileLast() {
	s.expectItems()
	avatar := repository.ProfileImageKey("profiles/alice/avatar/a1")
	s.profiles.EXPECT().Get(mock.Anything, "alice").
		Return(&repository.Profile{OwnerID: "alice", AvatarPath: &avatar}, nil)
	s.profileImages.EXPECT().Delete(mock.Anything, avatar).
		Run(func(context.Context, repository.ProfileImageKey) { s.record("avatar") }).Return(nil)
	s.profiles.EXPECT().Delete(mock.Anything).
		Run(func(context.Context) { s.record("profile") }).Return(nil)

	s.Require().NoError(s.account.Delete(s.T().Context(), "alice"))

	s.Equal("build", s.order[0])
	s.ElementsMatch([]string{"keyboard", "switch", "keycap set"}, s.order[1:4])
	s.Equal([]string{"avatar", "profile"}, s.order[4:])
}

func (s *DeleteSuite) TestNoProfile_StillDeletesItems() {
	s.expectItems()
	s.profiles.EXPECT().Get(mock.Anything, "alice").Return(nil, repository.ErrNotFound)

	s.Require().NoError(s.account.Delete(s.T().Context(), "alice"))

	s.Len(s.order, 4)
}

func (s *DeleteSuite) TestReadsEveryPage() {
	s.builds.EXPECT().List(mock.Anything, "alice", mock.Anything, "", mock.Anything, "").
		Return([]repository.Build{{ID: "b1"}}, "next", nil)
	s.builds.EXPECT().List(mock.Anything, "alice", mock.Anything, "", mock.Anything, "next").
		Return([]repository.Build{{ID: "b2"}}, "", nil)
	for _, id := range []string{"b1", "b2"} {
		s.builds.EXPECT().Get(mock.Anything, "alice", id).Return(&repository.Build{ID: id}, nil)
		s.builds.EXPECT().Delete(mock.Anything, id).
			Run(func(_ context.Context, id string) { s.record(id) }).Return(nil)
	}
	s.keyboards.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.switches.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.sets.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.profiles.EXPECT().Get(mock.Anything, "alice").Return(nil, repository.ErrNotFound)

	s.Require().NoError(s.account.Delete(s.T().Context(), "alice"))

	slices.Sort(s.order)
	s.Equal([]string{"b1", "b2"}, s.order)
}

func (s *DeleteSuite) TestListsEveryVisibility() {
	s.builds.EXPECT().List(mock.Anything, "alice", []repository.Visibility{
		repository.VisibilityPublic, repository.VisibilityAuthenticated, repository.VisibilityPrivate,
	}, "", mock.Anything, "").Return(nil, "", errors.New("stop"))

	s.Require().Error(s.account.Delete(s.T().Context(), "alice"))
}

func (s *DeleteSuite) TestItemDeleteFails_ProfileKept() {
	s.builds.EXPECT().List(mock.Anything, "alice", mock.Anything, "", mock.Anything, "").
		Return([]repository.Build{{ID: "b1"}}, "", nil)
	s.builds.EXPECT().Get(mock.Anything, "alice", "b1").Return(nil, errors.New("dynamo down"))

	s.Require().Error(s.account.Delete(s.T().Context(), "alice"))
	s.profiles.AssertNotCalled(s.T(), "Delete", mock.Anything)
}

func (s *DeleteSuite) TestProfileConflict_Matchable() {
	s.builds.EXPECT().List(mock.Anything, "alice", mock.Anything, "", mock.Anything, "").Return(nil, "", nil)
	s.keyboards.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.switches.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.sets.EXPECT().List(mock.Anything, "alice", mock.Anything, mock.Anything, "").Return(nil, "", nil)
	s.profiles.EXPECT().Get(mock.Anything, "alice").Return(&repository.Profile{OwnerID: "alice"}, nil)
	s.profiles.EXPECT().Delete(mock.Anything).Return(repository.ErrMutationConflict)

	s.ErrorIs(s.account.Delete(s.T().Context(), "alice"), repository.ErrMutationConflict)
}
