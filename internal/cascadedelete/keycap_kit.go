package cascadedelete

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// KeycapKitResult is a DeleteKeycapKit call's success return value.
// DeletedBuildIDs (via the embedded Result) is empty except in cascade
// mode. Image cleanup (both the kit's own and any cascaded builds')
// happens inside DeleteKeycapKit itself, before the corresponding DB
// record is deleted, so there's nothing left for the caller to clean up.
type KeycapKitResult struct {
	Result
}

// DeleteKeycapKit deletes the kit identified by (setID, kitID) from the
// caller's keycap set, applying onDelete's policy toward any build that
// still references it:
//
//   - OnDeleteBlock: if any build references (setID, kitID), deletes
//     nothing and returns a *BlockedError listing those build ids.
//   - OnDeleteCascade: deletes every build referencing (setID, kitID)
//     first, then the kit. Returns the deleted build ids.
//
// Every delete (the kit's own, and any cascaded build's) removes the
// item's S3 image object(s) before the DynamoDB record, not the reverse -
// this makes the whole operation safely retryable: S3's DeleteObject is
// idempotent, so a failure at either step (S3 or DB) can be retried to
// completion with nothing orphaned and no failure masked by a
// false-success retry.
//
// onDelete must be one of the two OnDeleteX values - see [ParseOnDelete].
// DeleteKeycapKit does no authorization itself; ownerID must already be the
// caller's own resolved subject.
func DeleteKeycapKit(
	ctx context.Context,
	keycapSetRepo repository.KeycapSetRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	kitImages repository.KeycapKitImageStore,
	ownerID, setID, kitID string,
	onDelete OnDelete,
) (KeycapKitResult, error) {
	switch onDelete {
	case OnDeleteBlock, OnDeleteCascade:
	default:
		return KeycapKitResult{}, fmt.Errorf("deleting keycap kit %q/%q: unknown on_delete value %q", setID, kitID, onDelete)
	}

	deleteKit := func() (KeycapKitResult, error) {
		ks, err := keycapSetRepo.Get(ctx, ownerID, setID)
		if err != nil {
			return KeycapKitResult{}, fmt.Errorf("getting keycap set %q for kit %q image cleanup: %w", setID, kitID, err)
		}
		kit, ok := ks.Kits[kitID]
		if !ok {
			return KeycapKitResult{}, nil
		}
		if kit.ImagePath != nil {
			if err := kitImages.Delete(ctx, *kit.ImagePath); err != nil {
				return KeycapKitResult{}, fmt.Errorf("deleting image for keycap kit %q/%q: %w", setID, kitID, err)
			}
		}
		if err := keycapSetRepo.DeleteKit(ctx, setID, kitID); err != nil {
			return KeycapKitResult{}, fmt.Errorf("deleting keycap kit %q/%q: %w", setID, kitID, err)
		}
		return KeycapKitResult{}, nil
	}

	buildIDs, err := buildRepo.FindBuildsReferencingKeycapKit(ctx, ownerID, setID, kitID)
	if err != nil {
		return KeycapKitResult{}, fmt.Errorf("finding builds referencing keycap kit %q/%q: %w", setID, kitID, err)
	}

	if onDelete == OnDeleteBlock {
		if len(buildIDs) > 0 {
			return KeycapKitResult{}, &BlockedError{BuildIDs: buildIDs}
		}
		return deleteKit()
	}

	errs := make([]error, len(buildIDs))
	var wg sync.WaitGroup
	for i, buildID := range buildIDs {
		wg.Add(1)
		go func(i int, buildID string) {
			defer wg.Done()

			b, err := buildRepo.Get(ctx, ownerID, buildID)
			if errors.Is(err, repository.ErrNotFound) {
				return
			}
			if err != nil {
				errs[i] = fmt.Errorf("getting build %q for image cleanup: %w", buildID, err)
				return
			}
			if err := deleteBuildImages(ctx, buildImages, b); err != nil {
				errs[i] = fmt.Errorf("deleting images for build %q referencing keycap kit %q/%q: %w", buildID, setID, kitID, err)
				return
			}
			if err := buildRepo.Delete(ctx, buildID); err != nil {
				errs[i] = fmt.Errorf("cascade-deleting build %q referencing keycap kit %q/%q: %w", buildID, setID, kitID, err)
			}
		}(i, buildID)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return KeycapKitResult{}, err
	}

	result, err := deleteKit()
	if err != nil {
		return KeycapKitResult{}, fmt.Errorf("deleting keycap kit %q/%q after cascading %d build(s): %w", setID, kitID, len(buildIDs), err)
	}
	result.DeletedBuildIDs = buildIDs

	return result, nil
}
