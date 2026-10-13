package repository

// ImageStatus is where an uploaded image stands in moderation. A new
// upload starts pending; the moderation Lambda moves it to approved or
// rejected, and an operator can override either.
type ImageStatus string

// The moderation statuses an image can have.
const (
	ImageStatusPending  ImageStatus = "pending"
	ImageStatusApproved ImageStatus = "approved"
	ImageStatusRejected ImageStatus = "rejected"
)

// Effective is s, or ImageStatusApproved when s is empty: images stored
// before moderation existed have no status.
func (s ImageStatus) Effective() ImageStatus {
	if s == "" {
		return ImageStatusApproved
	}
	return s
}

// VisibleTo reports whether an image with status s is shown to a viewer
// at all: its owner sees every image, anyone else only approved ones.
func (s ImageStatus) VisibleTo(isOwner bool) bool {
	return isOwner || s.Effective() == ImageStatusApproved
}

// Downloadable reports whether an image with status s may be served: a
// rejected image never is, even to its owner.
func (s ImageStatus) Downloadable() bool {
	return s.Effective() != ImageStatusRejected
}
