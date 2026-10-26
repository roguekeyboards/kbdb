package repoapi

import (
	"time"

	"github.com/rogueserenity/kbdb/internal/repository"
)

func signedExpiry() time.Time {
	return time.Now().Truncate(repository.ImageURLWindow).Add(repository.ImageURLLifetime)
}
