package authz

import (
	"context"
	"slices"

	apictx "github.com/roguekeyboards/mykeebs-api/internal/ctx"
	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// ReadableVisibilities reports which visibility tiers the caller identified
// by ctx (anonymous if no user ID is set) may read, for a route whose path
// names ownerID as the collection owner. Every mykeebs-api collection route is
// shaped /users/{userId}/..., so the owner being asked about is always known
// from the path before any repository query runs — callers scope the
// DynamoDB query (or a single-item fetch) to the returned set directly,
// rather than fetching everything and filtering per item after the fact.
//
//   - ownerID == caller's subject: all tiers (it's their own collection).
//   - no caller (anonymous): public only.
//   - caller present but ownerID differs: public + authenticated.
func ReadableVisibilities(ctx context.Context, ownerID string) []repository.Visibility {
	subject, _ := apictx.UserID(ctx)
	if subject != "" && subject == ownerID {
		return []repository.Visibility{
			repository.VisibilityPublic,
			repository.VisibilityAuthenticated,
			repository.VisibilityPrivate,
		}
	}

	if subject == "" {
		return []repository.Visibility{repository.VisibilityPublic}
	}

	return []repository.Visibility{
		repository.VisibilityPublic,
		repository.VisibilityAuthenticated,
	}
}

// IsOwner reports whether the caller identified by ctx is ownerID. Used to
// gate writes, which are always owner-only regardless of visibility.
func IsOwner(ctx context.Context, ownerID string) bool {
	subject, _ := apictx.UserID(ctx)
	return subject != "" && subject == ownerID
}

// CanReadVisibility reports whether visibility is in the caller's readable
// set for ownerID's collection (see ReadableVisibilities). Unlike List,
// which can scope its query to the whole readable set up front, a
// single-item fetch by exact key doesn't know the item's visibility until
// after it's fetched - this is the post-fetch check for that case.
func CanReadVisibility(ctx context.Context, ownerID string, visibility repository.Visibility) bool {
	return slices.Contains(ReadableVisibilities(ctx, ownerID), visibility)
}
