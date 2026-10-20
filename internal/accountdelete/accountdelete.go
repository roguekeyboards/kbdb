// Package accountdelete deletes a user's whole account: every image they
// own, every build, keyboard, switch and keycap set, then their profile.
// kbdb has no account apart from the profile, so the REST profile delete
// and MCP's delete_profile both call [Account.Delete].
package accountdelete

import (
	"context"
	"fmt"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// ItemStore is one table of the caller's items.
type ItemStore interface {
	DeleteAll(ctx context.Context) error
}

// Account holds what [Account.Delete] deletes from.
type Account struct {
	Images     repository.OwnerImageStore
	Builds     ItemStore
	Keyboards  ItemStore
	Switches   ItemStore
	KeycapSets ItemStore
	Profiles   repository.ProfileRepository
}

// Delete deletes everything the caller on ctx owns: images first, the
// profile last. Every step is idempotent, so a call that fails or times
// out partway can be retried to finish the rest.
// Errors wrap the repository's, so callers can still match
// [repository.ErrMutationConflict].
func (a Account) Delete(ctx context.Context) error {
	if err := a.Images.DeleteAll(ctx); err != nil {
		return fmt.Errorf("deleting images: %w", err)
	}
	for _, items := range []struct {
		name  string
		store ItemStore
	}{
		{"builds", a.Builds},
		{"keyboards", a.Keyboards},
		{"switches", a.Switches},
		{"keycap sets", a.KeycapSets},
	} {
		if err := items.store.DeleteAll(ctx); err != nil {
			return fmt.Errorf("deleting %s: %w", items.name, err)
		}
	}
	if err := a.Profiles.Delete(ctx); err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}
	return nil
}
