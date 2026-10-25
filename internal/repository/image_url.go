package repository

import "time"

const (
	// ImageURLWindow is the span within which every signed image GET URL
	// gets the same expiry. Signatures are deterministic, so the URL itself
	// is identical across the window and browsers cache the image.
	ImageURLWindow = 12 * time.Hour

	// ImageURLLifetime is how long a signed image GET URL lasts, counted
	// from the start of its window, so it has between
	// ImageURLLifetime-ImageURLWindow and ImageURLLifetime left when issued.
	ImageURLLifetime = 24 * time.Hour
)
