package authz_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/authz"
	kbdbctx "github.com/roguekeyboards/kbdb/internal/ctx"
	"github.com/roguekeyboards/kbdb/internal/repository"
)

func ctxFor(caller string) context.Context {
	ctx := context.Background()
	if caller != "" {
		ctx = kbdbctx.WithUserID(ctx, caller)
	}
	return ctx
}

type ReadableVisibilitiesSuite struct {
	suite.Suite
}

func TestReadableVisibilitiesSuite(t *testing.T) {
	suite.Run(t, new(ReadableVisibilitiesSuite))
}

func (s *ReadableVisibilitiesSuite) TestOwnerRequestingOwnCollection() {
	got := authz.ReadableVisibilities(ctxFor("alice"), "alice")
	s.ElementsMatch([]repository.Visibility{
		repository.VisibilityPublic, repository.VisibilityAuthenticated, repository.VisibilityPrivate,
	}, got)
}

func (s *ReadableVisibilitiesSuite) TestAnonymousRequestingSomeonesCollection() {
	got := authz.ReadableVisibilities(ctxFor(""), "alice")
	s.ElementsMatch([]repository.Visibility{repository.VisibilityPublic}, got)
}

func (s *ReadableVisibilitiesSuite) TestOtherAuthenticatedUserRequestingSomeonesCollection() {
	got := authz.ReadableVisibilities(ctxFor("bob"), "alice")
	s.ElementsMatch([]repository.Visibility{
		repository.VisibilityPublic, repository.VisibilityAuthenticated,
	}, got)
}

type CanReadVisibilitySuite struct {
	suite.Suite
}

func TestCanReadVisibilitySuite(t *testing.T) {
	suite.Run(t, new(CanReadVisibilitySuite))
}

func (s *CanReadVisibilitySuite) TestOwnerReadingOwnPrivateItem() {
	s.True(authz.CanReadVisibility(ctxFor("alice"), "alice", repository.VisibilityPrivate))
}

func (s *CanReadVisibilitySuite) TestAnonymousReadingPublicItem() {
	s.True(authz.CanReadVisibility(ctxFor(""), "alice", repository.VisibilityPublic))
}

func (s *CanReadVisibilitySuite) TestAnonymousReadingAuthenticatedItem() {
	s.False(authz.CanReadVisibility(ctxFor(""), "alice", repository.VisibilityAuthenticated))
}

func (s *CanReadVisibilitySuite) TestAnonymousReadingPrivateItem() {
	s.False(authz.CanReadVisibility(ctxFor(""), "alice", repository.VisibilityPrivate))
}

func (s *CanReadVisibilitySuite) TestOtherUserReadingAuthenticatedItem() {
	s.True(authz.CanReadVisibility(ctxFor("bob"), "alice", repository.VisibilityAuthenticated))
}

func (s *CanReadVisibilitySuite) TestOtherUserReadingPrivateItem() {
	s.False(authz.CanReadVisibility(ctxFor("bob"), "alice", repository.VisibilityPrivate))
}

type IsOwnerSuite struct {
	suite.Suite
}

func TestIsOwnerSuite(t *testing.T) {
	suite.Run(t, new(IsOwnerSuite))
}

func (s *IsOwnerSuite) TestOwner() {
	s.True(authz.IsOwner(ctxFor("alice"), "alice"))
}

func (s *IsOwnerSuite) TestOtherUser() {
	s.False(authz.IsOwner(ctxFor("bob"), "alice"))
}

func (s *IsOwnerSuite) TestAnonymous() {
	s.False(authz.IsOwner(ctxFor(""), "alice"))
}
