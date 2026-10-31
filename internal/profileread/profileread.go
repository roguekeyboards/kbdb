// Package profileread resolves a profile identifier (an IdP subject or a
// username) to a profile and applies the discoverable-or-owner visibility
// rule, shared by internal/handlers and internal/mcp.
package profileread

import (
	"context"
	"errors"
	"strings"

	apictx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// Resolve looks up the profile named by identifier (subject first, username
// fallback) and returns it only if discoverable or owned by the caller
// (from ctx). "Not found or not visible" is (nil, false, nil) so callers
// 404 without leaking whether a non-discoverable profile exists; a non-nil
// error is a real store failure.
func Resolve(ctx context.Context, repo repository.ProfileRepository, identifier string) (*repository.Profile, bool, error) {
	p, found, err := resolveProfile(ctx, repo, identifier)
	if err != nil || !found {
		return nil, false, err
	}

	if p.Discoverable {
		return p, true, nil
	}
	if caller, ok := apictx.UserID(ctx); ok && caller == p.OwnerID {
		return p, true, nil
	}

	return nil, false, nil
}

// resolveProfile fetches the profile named by identifier (subject first,
// then username), with no visibility check. A stale username claim pointing
// at a subject with no profile is (nil, false, nil), not a 500.
func resolveProfile(ctx context.Context, repo repository.ProfileRepository, identifier string) (*repository.Profile, bool, error) {
	p, err := repo.Get(ctx, identifier)
	if err == nil {
		return p, true, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}

	subject, err := repo.ResolveUsername(ctx, strings.ToLower(identifier))
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	p, err = repo.Get(ctx, subject)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return p, true, nil
}
