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
