// Package accountdelete deletes a user's whole account: every build,
// keyboard, switch and keycap set they own with its images, then their
// profile and avatar. kbdb has no account apart from the profile, so the
// REST profile delete and MCP's delete_profile both call [Account.Delete].
package accountdelete

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/rogueserenity/kbdb/internal/cascadedelete"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// Account holds what [Account.Delete] deletes from.
type Account struct {
	Profiles       repository.ProfileRepository
	ProfileImages  repository.ProfileImageStore
	Builds         repository.BuildRepository
	BuildImages    repository.BuildImageStore
	Keyboards      repository.KeyboardRepository
	KeyboardImages repository.KeyboardImageStore
	Switches       repository.SwitchRepository
	SwitchImages   repository.SwitchImageStore
	KeycapSets     repository.KeycapSetRepository
	KitImages      repository.KeycapKitImageStore
}

// pageSize is the List page size used to collect item IDs.
const pageSize = 100

// parallelDeletes bounds concurrent item deletes, so a large collection
// fits in one request more often without flooding DynamoDB or S3.
const parallelDeletes = 8

var allVisibilities = []repository.Visibility{
	repository.VisibilityPublic,
	repository.VisibilityAuthenticated,
	repository.VisibilityPrivate,
}

// Delete deletes everything ownerID owns. Builds go first, so no other
// item is still referenced by one; the profile goes last. Each item is
// deleted in full (images, then record) before it's gone from its list, so
// a call that fails or times out partway can be retried to finish the rest,
// including after the profile is already gone. ownerID must be the caller's
// own subject, also on ctx: the repositories' Delete methods act on the
// caller's items. Errors wrap the repository's, so callers can still match
// [repository.ErrMutationConflict].
func (a Account) Delete(ctx context.Context, ownerID string) error {
	buildIDs, err := collect(func(cursor string) ([]repository.Build, string, error) {
		return a.Builds.List(ctx, ownerID, allVisibilities, "", pageSize, cursor)
	}, func(b repository.Build) string { return b.ID })
	if err != nil {
		return fmt.Errorf("listing builds: %w", err)
	}
	if err := deleteAll(ctx, buildIDs, func(ctx context.Context, id string) error {
		return cascadedelete.DeleteBuild(ctx, a.Builds, a.BuildImages, ownerID, id)
	}); err != nil {
		return err
	}

	keyboardIDs, err := collect(func(cursor string) ([]repository.Keyboard, string, error) {
		return a.Keyboards.List(ctx, ownerID, allVisibilities, pageSize, cursor)
	}, func(k repository.Keyboard) string { return k.ID })
	if err != nil {
		return fmt.Errorf("listing keyboards: %w", err)
	}
	switchIDs, err := collect(func(cursor string) ([]repository.Switch, string, error) {
		return a.Switches.List(ctx, ownerID, allVisibilities, pageSize, cursor)
	}, func(s repository.Switch) string { return s.ID })
	if err != nil {
		return fmt.Errorf("listing switches: %w", err)
	}
	setIDs, err := collect(func(cursor string) ([]repository.KeycapSet, string, error) {
		return a.KeycapSets.List(ctx, ownerID, allVisibilities, pageSize, cursor)
	}, func(s repository.KeycapSet) string { return s.ID })
	if err != nil {
		return fmt.Errorf("listing keycap sets: %w", err)
	}

	// Cascade, not block: a build created since the sweep above is deleted
	// with the item it references rather than failing the whole delete.
	if err := deleteAll(ctx, keyboardIDs, func(ctx context.Context, id string) error {
		_, err := cascadedelete.DeleteKeyboard(ctx, a.Keyboards, a.Builds, a.BuildImages, a.KeyboardImages, ownerID, id, cascadedelete.OnDeleteCascade)
		return err
	}); err != nil {
		return err
	}
	if err := deleteAll(ctx, switchIDs, func(ctx context.Context, id string) error {
		_, err := cascadedelete.DeleteSwitch(ctx, a.Switches, a.Builds, a.BuildImages, a.SwitchImages, ownerID, id, cascadedelete.OnDeleteCascade)
		return err
	}); err != nil {
		return err
	}
	if err := deleteAll(ctx, setIDs, func(ctx context.Context, id string) error {
		_, err := cascadedelete.DeleteKeycapSet(ctx, a.KeycapSets, a.Builds, a.BuildImages, a.KitImages, ownerID, id, cascadedelete.OnDeleteCascade)
		return err
	}); err != nil {
		return err
	}

	return a.deleteProfile(ctx, ownerID)
}

func (a Account) deleteProfile(ctx context.Context, ownerID string) error {
	p, err := a.Profiles.Get(ctx, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting profile: %w", err)
	}
	if p.AvatarPath != nil {
		if err := a.ProfileImages.Delete(ctx, *p.AvatarPath); err != nil {
			return fmt.Errorf("deleting profile avatar: %w", err)
		}
	}
	if err := a.Profiles.Delete(ctx); err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}
	return nil
}

func collect[T any](list func(cursor string) ([]T, string, error), id func(T) string) ([]string, error) {
	var ids []string
	cursor := ""
	for {
		items, next, err := list(cursor)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			ids = append(ids, id(item))
		}
		if next == "" {
			return ids, nil
		}
		cursor = next
	}
}

func deleteAll(ctx context.Context, ids []string, del func(context.Context, string) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(parallelDeletes)
	for _, id := range ids {
		g.Go(func() error { return del(ctx, id) })
	}
	return g.Wait()
}
