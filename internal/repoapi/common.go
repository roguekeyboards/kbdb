package repoapi

import (
	"fmt"
	"math"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rogueserenity/kbdb/internal/handlers/api"
	"github.com/rogueserenity/kbdb/internal/repository"
)

// dateLayout matches how openapi_types.Date marshals/unmarshals.
const dateLayout = "2006-01-02"

func parseAPIDate(s string) (*openapi_types.Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, fmt.Errorf("stored date %q does not match layout %q: %w", s, dateLayout, err)
	}

	return &openapi_types.Date{Time: t}, nil
}

// getURLRefreshFloor is how long a cached image URL must have left to be
// reused. A URL expires 24h after the start of the 12h window it was signed
// in, so more than 12h left means it was signed in the current window.
const getURLRefreshFloor = 12 * time.Hour

// resolveImageURL returns cachedURL if it has more than getURLRefreshFloor
// left before cachedExpiresAt, and otherwise signs a new one with mint and
// stores it with writeBack. A writeBack failure isn't an error: the new URL
// is returned regardless.
func resolveImageURL(
	cachedURL *string,
	cachedExpiresAt *time.Time,
	mint func() (url string, expiresAt time.Time, err error),
	writeBack func(url string, expiresAt time.Time) error,
) (string, error) {
	if cachedURL != nil && cachedExpiresAt != nil && time.Until(*cachedExpiresAt) > getURLRefreshFloor {
		return *cachedURL, nil
	}

	url, expiresAt, err := mint()
	if err != nil {
		return "", err
	}

	_ = writeBack(url, expiresAt)

	return url, nil
}

// sumKnownCosts sums the non-nil components, treating a nil one as
// excluded rather than zero. Returns nil if none are set.
func sumKnownCosts(components ...*float64) *float64 {
	var total float64
	var haveAny bool
	for _, c := range components {
		if c == nil {
			continue
		}
		total += *c
		haveAny = true
	}
	if !haveAny {
		return nil //nolint:nilnil // no known-priced components is a valid, expected result
	}

	return &total
}

// roundCents rounds a cost to whole cents.
func roundCents(cost *float64) *float64 {
	if cost == nil {
		return nil
	}
	rounded := math.Round(*cost*100) / 100
	return &rounded
}

// ownerVisibility returns v for the item's owner and nil for anyone else.
func ownerVisibility(v repository.Visibility, isOwner bool) *api.Visibility {
	if !isOwner {
		return nil
	}
	out := api.Visibility(v)

	return &out
}

// imageStatusToAPI is the moderation status and rejection reason the
// caller sees: both are the owner's alone.
func imageStatusToAPI(status repository.ImageStatus, reason *string, isOwner bool) (*api.ImageStatus, *string) {
	if !isOwner {
		return nil, nil
	}
	out := api.ImageStatus(status.Effective())

	return &out, reason
}

// visibleImages keeps the images the caller may see - see
// [repository.ImageStatus.VisibleTo].
func visibleImages[T any](images []T, statusOf func(T) repository.ImageStatus, isOwner bool) []T {
	var out []T
	for _, img := range images {
		if statusOf(img).VisibleTo(isOwner) {
			out = append(out, img)
		}
	}

	return out
}
