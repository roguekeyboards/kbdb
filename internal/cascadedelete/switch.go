package cascadedelete

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// SwitchResult is a DeleteSwitch call's success return value.
// DeletedBuildIDs (via the embedded Result) is empty except in cascade
// mode. Image cleanup (both the switch's own and any cascaded builds')
// happens inside DeleteSwitch itself, before the corresponding DB record
// is deleted, so there's nothing left for the caller to clean up.
type SwitchResult struct {
	Result
}

// DeleteSwitch deletes the switch identified by (ownerID, switchID),
// applying onDelete's policy toward any build that still references it:
//
//   - OnDeleteBlock: if any build references switchID, deletes nothing and
//     returns a *BlockedError listing those build ids.
//   - OnDeleteCascade: deletes every build referencing switchID first, then
//     the switch. Returns the deleted build ids.
//
// Every delete (the switch's own, and any cascaded build's) removes the
// item's S3 image object(s) before the DynamoDB record, not the reverse -
// this makes the whole operation safely retryable: S3's DeleteObject is
// idempotent, so a failure at either step (S3 or DB) can be retried to
// completion with nothing orphaned and no failure masked by a
// false-success retry.
//
// onDelete must be one of the two OnDeleteX values - see [ParseOnDelete].
// DeleteSwitch does no authorization itself; ownerID must already be the
// caller's own resolved subject.
func DeleteSwitch(
	ctx context.Context,
	switchRepo repository.SwitchRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	switchImages repository.SwitchImageStore,
	ownerID, switchID string,
	onDelete OnDelete,
) (SwitchResult, error) {
	switch onDelete {
	case OnDeleteBlock, OnDeleteCascade:
	default:
		return SwitchResult{}, fmt.Errorf("deleting switch %q: unknown on_delete value %q", switchID, onDelete)
	}

	deleteSwitch := func() (SwitchResult, error) {
		sw, err := switchRepo.Get(ctx, ownerID, switchID)
		if errors.Is(err, repository.ErrNotFound) {
			return SwitchResult{}, nil
		}
		if err != nil {
			return SwitchResult{}, fmt.Errorf("getting switch %q for image cleanup: %w", switchID, err)
		}
		if sw.ImagePath != nil {
			if err := switchImages.Delete(ctx, *sw.ImagePath); err != nil {
				return SwitchResult{}, fmt.Errorf("deleting image for switch %q: %w", switchID, err)
			}
		}
		if err := switchRepo.Delete(ctx, switchID); err != nil {
			return SwitchResult{}, fmt.Errorf("deleting switch %q: %w", switchID, err)
		}
		return SwitchResult{}, nil
	}

	buildIDs, err := buildRepo.FindBuildsReferencingSwitch(ctx, ownerID, switchID)
	if err != nil {
		return SwitchResult{}, fmt.Errorf("finding builds referencing switch %q: %w", switchID, err)
	}

	if onDelete == OnDeleteBlock {
		if len(buildIDs) > 0 {
			return SwitchResult{}, &BlockedError{BuildIDs: buildIDs}
		}
		return deleteSwitch()
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
				errs[i] = fmt.Errorf("deleting images for build %q referencing switch %q: %w", buildID, switchID, err)
				return
			}
			if err := buildRepo.Delete(ctx, buildID); err != nil {
				errs[i] = fmt.Errorf("cascade-deleting build %q referencing switch %q: %w", buildID, switchID, err)
			}
		}(i, buildID)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return SwitchResult{}, err
	}

	result, err := deleteSwitch()
	if err != nil {
		return SwitchResult{}, fmt.Errorf("deleting switch %q after cascading %d build(s): %w", switchID, len(buildIDs), err)
	}
	result.DeletedBuildIDs = buildIDs

	return result, nil
}
