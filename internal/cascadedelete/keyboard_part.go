package cascadedelete

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// DeleteKeyboardPlate deletes the plate identified by (keyboardID, plateID)
// from the caller's keyboard, applying onDelete's policy toward any build
// whose plate is plateID - see [DeleteKeycapKit] for the block/cascade
// semantics. A plate has no image, so only cascaded builds' images are
// cleaned up. Returns ErrNotFound (wrapped) if the keyboard doesn't exist.
func DeleteKeyboardPlate(
	ctx context.Context,
	keyboardRepo repository.KeyboardRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	ownerID, keyboardID, plateID string,
	onDelete OnDelete,
) (Result, error) {
	uses := func(b *repository.Build) bool { return b.Plate != nil && *b.Plate == plateID }
	del := func() error { return keyboardRepo.DeletePlate(ctx, keyboardID, plateID) }
	return deleteKeyboardPart(ctx, buildRepo, buildImages, ownerID, keyboardID, "plate", plateID, onDelete, uses, del)
}

// DeleteKeyboardPCB is DeleteKeyboardPlate for PCBs.
func DeleteKeyboardPCB(
	ctx context.Context,
	keyboardRepo repository.KeyboardRepository,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	ownerID, keyboardID, pcbID string,
	onDelete OnDelete,
) (Result, error) {
	uses := func(b *repository.Build) bool { return b.PCB != nil && *b.PCB == pcbID }
	del := func() error { return keyboardRepo.DeletePCB(ctx, keyboardID, pcbID) }
	return deleteKeyboardPart(ctx, buildRepo, buildImages, ownerID, keyboardID, "PCB", pcbID, onDelete, uses, del)
}

// deleteKeyboardPart finds the part's builds through the keyboard's
// reverse-reference markers - a part has none of its own - keeping those
// that uses reports, then blocks or cascades like the other DeleteX calls.
func deleteKeyboardPart(
	ctx context.Context,
	buildRepo repository.BuildRepository,
	buildImages repository.BuildImageStore,
	ownerID, keyboardID, kind, partID string,
	onDelete OnDelete,
	uses func(*repository.Build) bool,
	del func() error,
) (Result, error) {
	switch onDelete {
	case OnDeleteBlock, OnDeleteCascade:
	default:
		return Result{}, fmt.Errorf("deleting keyboard %s %q/%q: unknown on_delete value %q", kind, keyboardID, partID, onDelete)
	}

	candidateIDs, err := buildRepo.FindBuildsReferencingKeyboard(ctx, ownerID, keyboardID)
	if err != nil {
		return Result{}, fmt.Errorf("finding builds referencing keyboard %q: %w", keyboardID, err)
	}

	fetched := make([]*repository.Build, len(candidateIDs))
	fetchErrs := make([]error, len(candidateIDs))
	var fetchWG sync.WaitGroup
	for i, id := range candidateIDs {
		fetchWG.Add(1)
		go func(i int, id string) {
			defer fetchWG.Done()

			b, err := buildRepo.Get(ctx, ownerID, id)
			if errors.Is(err, repository.ErrNotFound) {
				return
			}
			if err != nil {
				fetchErrs[i] = fmt.Errorf("getting build %q: %w", id, err)
				return
			}
			fetched[i] = b
		}(i, id)
	}
	fetchWG.Wait()
	if err := errors.Join(fetchErrs...); err != nil {
		return Result{}, err
	}

	var builds []*repository.Build
	for _, b := range fetched {
		if b != nil && uses(b) {
			builds = append(builds, b)
		}
	}
	buildIDs := make([]string, len(builds))
	for i, b := range builds {
		buildIDs[i] = b.ID
	}

	if onDelete == OnDeleteBlock && len(buildIDs) > 0 {
		return Result{}, &BlockedError{BuildIDs: buildIDs}
	}

	errs := make([]error, len(builds))
	var wg sync.WaitGroup
	for i, b := range builds {
		wg.Add(1)
		go func(i int, b *repository.Build) {
			defer wg.Done()

			if err := deleteBuildImages(ctx, buildImages, b); err != nil {
				errs[i] = fmt.Errorf("deleting images for build %q using keyboard %s %q: %w", b.ID, kind, partID, err)
				return
			}
			if err := buildRepo.Delete(ctx, b.ID); err != nil {
				errs[i] = fmt.Errorf("cascade-deleting build %q using keyboard %s %q: %w", b.ID, kind, partID, err)
			}
		}(i, b)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return Result{}, err
	}

	if err := del(); err != nil {
		return Result{}, fmt.Errorf("deleting keyboard %s %q/%q: %w", kind, keyboardID, partID, err)
	}

	if onDelete == OnDeleteCascade {
		return Result{DeletedBuildIDs: buildIDs}, nil
	}
	return Result{}, nil
}
