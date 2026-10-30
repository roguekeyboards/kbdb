package profileread_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/profileread"
	"github.com/roguekeyboards/kbdb/internal/repository"
	"github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

type ResolveSuite struct {
	suite.Suite

	repo *mocks.MockProfileRepository
}

func TestResolveSuite(t *testing.T) {
	suite.Run(t, new(ResolveSuite))
}

func (s *ResolveSuite) SetupTest() {
	s.repo = mocks.NewMockProfileRepository(s.T())
}

func (s *ResolveSuite) TestByID_Discoverable_Returned() {
	s.repo.EXPECT().Get(s.T().Context(), "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: true}, nil)

	p, ok, err := profileread.Resolve(s.T().Context(), s.repo, "user-alice")

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", p.Username)
}

func (s *ResolveSuite) TestByUsername_Discoverable_Returned() {
	s.repo.EXPECT().Get(s.T().Context(), "alice").Return(nil, repository.ErrNotFound)
	s.repo.EXPECT().ResolveUsername(s.T().Context(), "alice").Return("user-alice", nil)
	s.repo.EXPECT().Get(s.T().Context(), "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: true}, nil)

	p, ok, err := profileread.Resolve(s.T().Context(), s.repo, "alice")

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", p.Username)
}

func (s *ResolveSuite) TestByUsername_MixedCaseIdentifier_LowercasedForClaimLookup() {
	s.repo.EXPECT().Get(s.T().Context(), "Alice").Return(nil, repository.ErrNotFound)
	s.repo.EXPECT().ResolveUsername(s.T().Context(), "alice").Return("user-alice", nil)
	s.repo.EXPECT().Get(s.T().Context(), "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: true}, nil)

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "Alice")

	s.Require().NoError(err)
	s.True(ok)
}

func (s *ResolveSuite) TestNonDiscoverable_Owner_Returned() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "user-alice")
	s.repo.EXPECT().Get(ctx, "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: false}, nil)

	p, ok, err := profileread.Resolve(ctx, s.repo, "user-alice")

	s.Require().NoError(err)
	s.True(ok)
	s.Equal("alice", p.Username)
}

func (s *ResolveSuite) TestNonDiscoverable_OtherCaller_NotFound() {
	ctx := kbdbctx.WithUserID(s.T().Context(), "user-bob")
	s.repo.EXPECT().Get(ctx, "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: false}, nil)

	p, ok, err := profileread.Resolve(ctx, s.repo, "user-alice")

	s.Require().NoError(err)
	s.False(ok)
	s.Nil(p)
}

func (s *ResolveSuite) TestNonDiscoverable_Anonymous_NotFound() {
	s.repo.EXPECT().Get(s.T().Context(), "user-alice").
		Return(&repository.Profile{OwnerID: "user-alice", Username: "alice", Discoverable: false}, nil)

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "user-alice")

	s.Require().NoError(err)
	s.False(ok)
}

func (s *ResolveSuite) TestNoSuchIDOrUsername_NotFound() {
	s.repo.EXPECT().Get(s.T().Context(), "ghost").Return(nil, repository.ErrNotFound)
	s.repo.EXPECT().ResolveUsername(s.T().Context(), "ghost").Return("", repository.ErrNotFound)

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "ghost")

	s.Require().NoError(err)
	s.False(ok)
}

func (s *ResolveSuite) TestStaleClaim_NoProfile_NotFoundNotError() {
	s.repo.EXPECT().Get(s.T().Context(), "stale").Return(nil, repository.ErrNotFound)
	s.repo.EXPECT().ResolveUsername(s.T().Context(), "stale").Return("user-gone", nil)
	s.repo.EXPECT().Get(s.T().Context(), "user-gone").Return(nil, repository.ErrNotFound)

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "stale")

	s.Require().NoError(err)
	s.False(ok)
}

func (s *ResolveSuite) TestStoreError_Propagates() {
	s.repo.EXPECT().Get(s.T().Context(), "user-alice").Return(nil, errors.New("dynamo down"))

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "user-alice")

	s.Require().Error(err)
	s.False(ok)
}

func (s *ResolveSuite) TestResolveUsernameError_Propagates() {
	s.repo.EXPECT().Get(s.T().Context(), "alice").Return(nil, repository.ErrNotFound)
	s.repo.EXPECT().ResolveUsername(s.T().Context(), "alice").Return("", errors.New("dynamo down"))

	_, ok, err := profileread.Resolve(s.T().Context(), s.repo, "alice")

	s.Require().Error(err)
	s.False(ok)
}
