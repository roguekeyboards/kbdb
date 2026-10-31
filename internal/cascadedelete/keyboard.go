package cascadedelete

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// KeyboardResult is a DeleteKeyboard call's success return value.
// DeletedBuildIDs (via the embedded Result) is empty except in cascade
// mode. Image cleanup (both the keyboard's own and any cascaded builds')
// happens inside DeleteKeyboard itself, before the corresponding DB
// record is deleted, so there's nothing left for the caller to clean up.
type KeyboardResult struct {
	Result
}

// deleteBuildImages - see [DeleteKeyboard]'s doc comment for why this
// gates the build's own DB delete.
func deleteBuildImages(ctx context.Context, images repository.BuildImageStore, build *repository.Build) error {
	errs := make([]error, 0, len(build.Images))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, entry := range build.Images {
		wg.Add(1)
		go func(key repository.BuildImageKey) {
			defer wg.Done()
			if err := images.DeleteBuildImage(ctx, key); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(entry.Path)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// deleteKeyboardImages - see [DeleteKeyboard]'s doc comment for why this
// gates the keyboard's own DB delete.
func deleteKeyboardImages(ctx context.Context, images repository.KeyboardImageStore, kb *repository.Keyboard) error {
	errs := make([]error, 0, len(kb.Images))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, entry := range kb.Images {
		wg.Add(1)
		go func(key repository.KeyboardImageKey) {
			defer wg.Done()
			if err := images.DeleteKeyboardImage(ctx, key); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(entry.Path)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// DeleteKeyboard deletes the keyboard identified by (ownerID, keyboardID),
// applying onDelete's policy toward any build that still references it:
//
//   - OnDeleteBlock: if any build references keyboardID, deletes nothing
//     and returns a *BlockedError listing those build ids.
//   - OnDeleteCascade: deletes every build referencing keyboardID first,
//     then the keyboard. Returns the deleted build ids.
//
// Every delete (the keyboard's own, and any cascaded build's) removes the
// item's S3 image object(s) before the DynamoDB record, not the reverse -
// this makes the whole operation safely retryable: S3's DeleteObject is
// idempotent, so a failure at either step (S3 or DB) can be retried to
// completion with nothing orphaned and no failure masked by a
// false-success retry.
//
// onDelete must be one of the two OnDeleteX values - see [ParseOnDelete].
// DeleteKeyboard does no authorization itself; ownerID must already be the
// caller's own resolved subject.
func DeleteKeyboard(
	ctx context.Context,
	keyboardRepo repository.KeyboardRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	keyboardImages repository.KeyboardImageStore,
	ownerID, keyboardID string,
	onDelete OnDelete,
) (KeyboardResult, error) {
	switch onDelete {
	case OnDeleteBlock, OnDeleteCascade:
	default:
		return KeyboardResult{}, fmt.Errorf("deleting keyboard %q: unknown on_delete value %q", keyboardID, onDelete)
	}

	deleteKeyboard := func() (KeyboardResult, error) {
		kb, err := keyboardRepo.Get(ctx, ownerID, keyboardID)
		if errors.Is(err, repository.ErrNotFound) {
			return KeyboardResult{}, nil
		}
		if err != nil {
			return KeyboardResult{}, fmt.Errorf("getting keyboard %q for image cleanup: %w", keyboardID, err)
		}
		if err := deleteKeyboardImages(ctx, keyboardImages, kb); err != nil {
			return KeyboardResult{}, fmt.Errorf("deleting images for keyboard %q: %w", keyboardID, err)
		}
		if err := keyboardRepo.Delete(ctx, keyboardID); err != nil {
			return KeyboardResult{}, fmt.Errorf("deleting keyboard %q: %w", keyboardID, err)
		}
		return KeyboardResult{}, nil
	}

	buildIDs, err := buildRepo.FindBuildsReferencingKeyboard(ctx, ownerID, keyboardID)
	if err != nil {
		return KeyboardResult{}, fmt.Errorf("finding builds referencing keyboard %q: %w", keyboardID, err)
	}

	if onDelete == OnDeleteBlock {
		if len(buildIDs) > 0 {
			return KeyboardResult{}, &BlockedError{BuildIDs: buildIDs}
		}
		return deleteKeyboard()
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
				errs[i] = fmt.Errorf("deleting images for build %q referencing keyboard %q: %w", buildID, keyboardID, err)
				return
			}
			if err := buildRepo.Delete(ctx, buildID); err != nil {
				errs[i] = fmt.Errorf("cascade-deleting build %q referencing keyboard %q: %w", buildID, keyboardID, err)
			}
		}(i, buildID)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return KeyboardResult{}, err
	}

	result, err := deleteKeyboard()
	if err != nil {
		return KeyboardResult{}, fmt.Errorf("deleting keyboard %q after cascading %d build(s): %w", keyboardID, len(buildIDs), err)
	}
	result.DeletedBuildIDs = buildIDs

	return result, nil
}
