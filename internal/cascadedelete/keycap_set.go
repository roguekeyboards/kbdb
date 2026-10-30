package cascadedelete

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

// KeycapSetResult is a DeleteKeycapSet call's success return value.
// DeletedBuildIDs (via the embedded Result) is empty except in cascade
// mode. Image cleanup (both the set's own kits' and any cascaded builds')
// happens inside DeleteKeycapSet itself, before the corresponding DB
// record is deleted, so there's nothing left for the caller to clean up.
type KeycapSetResult struct {
	Result
}

// deleteKeycapSetImages - see [DeleteKeycapSet]'s doc comment for why this
// gates the set's own DB delete.
func deleteKeycapSetImages(ctx context.Context, images repository.KeycapKitImageStore, ks *repository.KeycapSet) error {
	kits := make([]repository.KeycapKit, 0, len(ks.Kits))
	for _, kit := range ks.Kits {
		kits = append(kits, kit)
	}

	errs := make([]error, len(kits))
	var wg sync.WaitGroup
	for i, kit := range kits {
		if kit.ImagePath == nil {
			continue
		}
		wg.Add(1)
		go func(i int, key repository.KeycapKitImageKey) {
			defer wg.Done()
			if err := images.Delete(ctx, key); err != nil {
				errs[i] = err
			}
		}(i, *kit.ImagePath)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// DeleteKeycapSet deletes the keycap set identified by (ownerID, setID),
// applying onDelete's policy toward any build that still references any
// kit in the set:
//
//   - OnDeleteBlock: if any build references any kit in setID, deletes
//     nothing and returns a *BlockedError listing those build ids.
//   - OnDeleteCascade: deletes every build referencing any kit in setID
//     first, then the set. Returns the deleted build ids.
//
// Every delete (the set's own kits', and any cascaded build's) removes
// the item's S3 image object(s) before the DynamoDB record, not the
// reverse - this makes the whole operation safely retryable: S3's
// DeleteObject is idempotent, so a failure at either step (S3 or DB) can
// be retried to completion with nothing orphaned and no failure masked by
// a false-success retry.
//
// onDelete must be one of the two OnDeleteX values - see [ParseOnDelete].
// DeleteKeycapSet does no authorization itself; ownerID must already be the
// caller's own resolved subject.
func DeleteKeycapSet(
	ctx context.Context,
	keycapSetRepo repository.KeycapSetRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	kitImages repository.KeycapKitImageStore,
	ownerID, setID string,
	onDelete OnDelete,
) (KeycapSetResult, error) {
	switch onDelete {
	case OnDeleteBlock, OnDeleteCascade:
	default:
		return KeycapSetResult{}, fmt.Errorf("deleting keycap set %q: unknown on_delete value %q", setID, onDelete)
	}

	deleteSet := func() (KeycapSetResult, error) {
		ks, err := keycapSetRepo.Get(ctx, ownerID, setID)
		if errors.Is(err, repository.ErrNotFound) {
			return KeycapSetResult{}, nil
		}
		if err != nil {
			return KeycapSetResult{}, fmt.Errorf("getting keycap set %q for image cleanup: %w", setID, err)
		}
		if err := deleteKeycapSetImages(ctx, kitImages, ks); err != nil {
			return KeycapSetResult{}, fmt.Errorf("deleting images for keycap set %q: %w", setID, err)
		}
		if err := keycapSetRepo.Delete(ctx, setID); err != nil {
			return KeycapSetResult{}, fmt.Errorf("deleting keycap set %q: %w", setID, err)
		}
		return KeycapSetResult{}, nil
	}

	buildIDs, err := buildRepo.FindBuildsReferencingKeycapSet(ctx, ownerID, setID)
	if err != nil {
		return KeycapSetResult{}, fmt.Errorf("finding builds referencing keycap set %q: %w", setID, err)
	}

	if onDelete == OnDeleteBlock {
		if len(buildIDs) > 0 {
			return KeycapSetResult{}, &BlockedError{BuildIDs: buildIDs}
		}
		return deleteSet()
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
				errs[i] = fmt.Errorf("deleting images for build %q referencing keycap set %q: %w", buildID, setID, err)
				return
			}
			if err := buildRepo.Delete(ctx, buildID); err != nil {
				errs[i] = fmt.Errorf("cascade-deleting build %q referencing keycap set %q: %w", buildID, setID, err)
			}
		}(i, buildID)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return KeycapSetResult{}, err
	}

	result, err := deleteSet()
	if err != nil {
		return KeycapSetResult{}, fmt.Errorf("deleting keycap set %q after cascading %d build(s): %w", setID, len(buildIDs), err)
	}
	result.DeletedBuildIDs = buildIDs

	return result, nil
}
