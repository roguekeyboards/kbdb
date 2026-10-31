package ownerprefs

import (
	"context"
	"errors"

	"github.com/roguekeyboards/mykeebs-api/internal/repository"
)

// ErrMissing means the route isn't wrapped in
// [github.com/roguekeyboards/mykeebs-api/internal/middleware.OwnerPreferences].
var ErrMissing = errors.New("no owner preferences on context")

type key struct{}

// WithPreferences returns a context carrying prefs for [Get].
func WithPreferences(ctx context.Context, prefs repository.ProfilePreferences) context.Context {
	return context.WithValue(ctx, key{}, prefs)
}

// Get returns the preferences [WithPreferences] put on ctx.
func Get(ctx context.Context) (repository.ProfilePreferences, error) {
	prefs, ok := ctx.Value(key{}).(repository.ProfilePreferences)
	if !ok {
		return repository.ProfilePreferences{}, ErrMissing
	}

	return prefs, nil
}
