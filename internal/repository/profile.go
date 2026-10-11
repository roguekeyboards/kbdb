package repository

import (
	"context"
	"fmt"
	"time"

	kbdbctx "github.com/rogueserenity/kbdb/internal/ctx"
)

// ProfileLink is one entry in a profile's links list. Both fields are
// required and non-blank; validation lives in internal/profilevalidate.
type ProfileLink struct {
	Name string `dynamodbav:"name" json:"name"`
	URL  string `dynamodbav:"url"  json:"url"`
}

// ProfilePreferences holds a user's display preferences. Currency is
// display-only: kbdb does no conversion. ShowPriceToMe applies to lists
// only, since editing an item needs its current price.
type ProfilePreferences struct {
	Currency          string `dynamodbav:"currency"             json:"currency"`
	ShowPriceToMe     bool   `dynamodbav:"show_price_to_me"     json:"show_price_to_me"`
	ShowPriceToOthers bool   `dynamodbav:"show_price_to_others" json:"show_price_to_others"`
}

// ShowPriceSingle reports whether a single-item response shows prices.
func (p ProfilePreferences) ShowPriceSingle(isOwner bool) bool {
	return isOwner || p.ShowPriceToOthers
}

// ShowPriceSummary reports whether a list response shows prices.
func (p ProfilePreferences) ShowPriceSummary(isOwner bool) bool {
	if isOwner {
		return p.ShowPriceToMe
	}
	return p.ShowPriceToOthers
}

// CurrencyFor returns p.Currency when price is set.
func (p ProfilePreferences) CurrencyFor(price *float64) *string {
	if price == nil {
		return nil
	}

	return &p.Currency
}

// DefaultProfilePreferences mirrors ProfilePreferences' OpenAPI schema
// defaults - the Go zero value isn't usable directly, since ShowPriceToMe
// defaults true but its zero value is false.
func DefaultProfilePreferences() ProfilePreferences {
	return ProfilePreferences{
		Currency:          "USD",
		ShowPriceToMe:     true,
		ShowPriceToOthers: false,
	}
}

// Profile is a user's public identity - one per user, partitioned by the
// IdP subject (OwnerID), no sort key.
type Profile struct {
	OwnerID string `dynamodbav:"user_id" json:"-"`

	Username string `dynamodbav:"username" json:"username"`

	Discoverable bool `dynamodbav:"discoverable" json:"discoverable"`

	Preferences ProfilePreferences `dynamodbav:"preferences" json:"preferences"`

	DiscordUsername *string       `dynamodbav:"discord_username,omitempty" json:"discord_username,omitempty"`
	Bio             *string       `dynamodbav:"bio,omitempty" json:"bio,omitempty"`
	Links           []ProfileLink `dynamodbav:"links,omitempty" json:"links,omitempty"`

	// AvatarPath is managed only via the image endpoints; a body PUT carries
	// it forward from the stored item rather than touching it.
	AvatarPath *ProfileImageKey `dynamodbav:"avatar_path,omitempty" json:"-"`

	// AvatarStatus is AvatarPath's moderation status, empty on images
	// stored before moderation (see ImageStatus.Effective).
	// AvatarRejectionReason is set only when rejected.
	AvatarStatus          ImageStatus `dynamodbav:"avatar_status,omitempty" json:"-"`
	AvatarRejectionReason *string     `dynamodbav:"avatar_rejection_reason,omitempty" json:"-"`

	// GetURL/GetURLExpiresAt cache the last presigned GET URL for
	// AvatarPath. SetAvatarPath clears both when AvatarPath changes.
	GetURL          *string    `dynamodbav:"get_url,omitempty" json:"-"`
	GetURLExpiresAt *time.Time `dynamodbav:"get_url_expires_at,omitempty" json:"-"`

	// DiscoverablePK / DiscordPK are the constant ("1") partition keys of the
	// two sparse directory GSIs, written only when the profile belongs in
	// each index (DiscordPK additionally requires DiscordUsername != nil).
	// Left nil - and omitted - otherwise, which is what keeps non-discoverable
	// profiles out of the indexes without a filter expression.
	DiscoverablePK *string `dynamodbav:"discoverable_pk,omitempty" json:"-"`
	DiscordPK      *string `dynamodbav:"discord_pk,omitempty" json:"-"`
}

// PreferencesReader is the narrow slice of ProfileRepository that
// price-gating code outside the profile handlers depends on.
// ProfileRepository satisfies it structurally.
type PreferencesReader interface {
	GetPreferences(ctx context.Context, ownerID string) (ProfilePreferences, error)
}

// ProfileRepository provides access to profiles. Reads take an explicit
// ownerID (a profile is readable by anyone, subject to the caller's
// discoverable check); writes read the caller from ctx, since a user can
// only write their own profile.
type ProfileRepository interface {
	// Get returns the profile keyed by ownerID, or ErrNotFound. Applies
	// no visibility check - the caller does that against Discoverable.
	Get(ctx context.Context, ownerID string) (*Profile, error)

	// GetPreferences returns ownerID's preferences, or
	// DefaultProfilePreferences() (not ErrNotFound) if they have no
	// profile yet, so gating logic elsewhere always has a preference to
	// consult.
	GetPreferences(ctx context.Context, ownerID string) (ProfilePreferences, error)

	// ResolveUsername returns the IdP subject that owns username, or
	// ErrNotFound.
	ResolveUsername(ctx context.Context, username string) (ownerID string, err error)

	// Create writes p as the caller's profile plus the { username -> user_id }
	// claim, atomically. Returns ErrAlreadyExists (this user already has a
	// profile) or ErrUsernameTaken (the username belongs to someone else).
	Create(ctx context.Context, p Profile) (*Profile, error)

	// Update replaces the body-settable fields of the caller's profile
	// (username, discoverable, discord_username, bio, links); AvatarPath
	// carries forward from the stored item. An omitted body field is
	// cleared. Returns ErrNotFound if the caller has no profile, or
	// ErrUsernameTaken if a changed username is claimed by a different user.
	Update(ctx context.Context, p Profile) (*Profile, error)

	// Delete removes the caller's profile and its { username -> user_id }
	// claim, atomically. A missing profile is a no-op success (idempotent).
	// This is a leaf delete: nothing references a Profile, so there is no
	// cascade.
	Delete(ctx context.Context) error

	// ListPublic returns a cursor-paginated page of discoverable profiles.
	// At most one of usernamePrefix / discordPrefix may be set (the caller
	// enforces this), each a begins_with filter on its directory index.
	ListPublic(ctx context.Context, usernamePrefix, discordPrefix string, limit int, cursor string) ([]Profile, string, error)

	// SetAvatarPath sets the caller's profile's AvatarPath to key, with
	// AvatarStatus pending. Returns ErrNotFound if the caller has no
	// profile.
	SetAvatarPath(ctx context.Context, key ProfileImageKey) error

	// ClearAvatarPath clears the caller's profile's AvatarPath (and its
	// moderation status) and returns the key that was cleared, or nil if it
	// was already unset (idempotent, not an error). Returns ErrNotFound if the
	// caller has no profile.
	ClearAvatarPath(ctx context.Context) (*ProfileImageKey, error)

	// SetImageGetCache stores url/expiresAt as ownerID's cached avatar GET
	// URL, conditioned on AvatarPath still equalling forPath. Takes an
	// explicit ownerID, unlike the other Set*/Clear* methods here, since
	// this is called from read paths that may be viewing another user's
	// profile, not just self-writes. Returns ok=false (not an error) if
	// that condition fails or ownerID has no profile.
	SetImageGetCache(ctx context.Context, ownerID string, forPath ProfileImageKey, url string, expiresAt time.Time) (ok bool, err error)

	// SetAvatarStatus sets ownerID's avatar's moderation status to `to` and
	// its rejection reason to reason (cleared when nil), conditioned on
	// AvatarPath still equalling forPath and the status still being from (a
	// missing status counts as approved). Also drops any cached GET URL. Takes
	// an explicit ownerID: the moderation Lambda and operator CLI act on any
	// owner's images. Returns ok=false (not an error) if that condition fails
	// or the item doesn't exist.
	SetAvatarStatus(ctx context.Context, ownerID string, forPath ProfileImageKey, from, to ImageStatus, reason *string) (ok bool, err error)
}

// ProfileImageKey is the object key a profile's avatar is stored under.
type ProfileImageKey string

// NewProfileImageKey builds the object key for one upload of the caller's
// avatar. ownerID comes from ctx so a caller can't address another user's
// prefix. Each upload gets its own key because upload URLs are single-use:
// a replacement can't be written over the previous object.
func NewProfileImageKey(ctx context.Context, uploadID string) (ProfileImageKey, error) {
	ownerID, ok := kbdbctx.UserID(ctx)
	if !ok {
		return "", ErrNoUserID
	}

	return ProfileImageKey(fmt.Sprintf("profiles/%s/avatar/%s", ownerID, uploadID)), nil
}

// ProfileImageStore stores a profile's avatar object in a private object
// store, addressed by ProfileImageKey.
type ProfileImageStore interface {
	// PresignGet returns a presigned GET URL for key, and when it stops
	// working - bounded by the signing credentials, not configured.
	PresignGet(ctx context.Context, key ProfileImageKey) (url string, expiresAt time.Time, err error)

	// PresignPut returns a short-lived presigned PUT URL for key, locked to
	// contentType and size via the Content-Type and Content-Length the upload
	// must match.
	// Single-use: the upload must also send If-None-Match: *, so once an
	// object exists at key the URL can't write it again.
	PresignPut(ctx context.Context, key ProfileImageKey, contentType string, size int64) (url string, err error)

	// Delete removes the object at key. Idempotent, matching S3 DeleteObject.
	Delete(ctx context.Context, key ProfileImageKey) error
}
