package cascadedelete

import (
	"context"
	"errors"
	"fmt"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// DeleteBuild deletes the build identified by (ownerID, buildID), its
// images first, then the record, like [DeleteKeyboard]. Nothing references
// a build, so there's no on_delete policy. A build that's already gone is
// not an error.
func DeleteBuild(
	ctx context.Context,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	ownerID, buildID string,
) error {
	b, err := buildRepo.Get(ctx, ownerID, buildID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting build %q for image cleanup: %w", buildID, err)
	}
	if err := deleteBuildImages(ctx, buildImages, b); err != nil {
		return fmt.Errorf("deleting images for build %q: %w", buildID, err)
	}
	if err := buildRepo.Delete(ctx, buildID); err != nil {
		return fmt.Errorf("deleting build %q: %w", buildID, err)
	}
	return nil
}
