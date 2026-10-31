package repository

// Visibility is who may read an item via its GET routes. Create/update/
// delete always require the caller to be the item's owner, regardless of
// visibility — see [github.com/roguekeyboards/mykeebs-api/internal/authz].
type Visibility string

const (
	// VisibilityPublic items are readable with no authentication at all.
	VisibilityPublic Visibility = "public"
	// VisibilityAuthenticated items are readable by any signed-in user,
	// but not anonymous callers.
	VisibilityAuthenticated Visibility = "authenticated"
	// VisibilityPrivate items are readable only by their owner.
	VisibilityPrivate Visibility = "private"
)

// Valid reports whether v is one of the three defined visibility tiers.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPublic, VisibilityAuthenticated, VisibilityPrivate:
		return true
	default:
		return false
	}
}
