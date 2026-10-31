package db

import (
	"context"
	"errors"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
)

// SeedProfileOptions is the fixture shape for SeedProfile. Only Username is
// required.
type SeedProfileOptions struct {
	Username        string
	Discoverable    bool
	DiscordUsername string
	Bio             string
	Links           []map[string]string
	AvatarPath      string
	Preferences     map[string]any
}

// SeedProfile writes a profile item and its { username -> user_id } claim
// directly into DynamoDB, bypassing the API. Discoverable also sets the
// sparse-GSI discriminators the way the app would; version is 0, as Create
// sets it.
func SeedProfile(ctx context.Context, ownerID string, opts SeedProfileOptions) error {
	profileTable := NewDynamoTable(ctx, support.ProfileTableName())

	item := map[string]any{
		"user_id":      ownerID,
		"username":     opts.Username,
		"discoverable": opts.Discoverable,
		"version":      0,
	}
	if opts.DiscordUsername != "" {
		item["discord_username"] = opts.DiscordUsername
	}
	if opts.Bio != "" {
		item["bio"] = opts.Bio
	}
	if len(opts.Links) > 0 {
		item["links"] = opts.Links
	}
	if opts.AvatarPath != "" {
		item["avatar_path"] = opts.AvatarPath
	}
	if opts.Preferences != nil {
		item["preferences"] = opts.Preferences
	}
	if opts.Discoverable {
		item["discoverable_pk"] = "1"
		if opts.DiscordUsername != "" {
			item["discord_pk"] = "1"
		}
	}

	if err := profileTable.PutItem(ctx, item); err != nil {
		return err
	}

	usernameTable := NewDynamoTable(ctx, support.ProfileUsernameTableName())
	return usernameTable.PutItem(ctx, map[string]any{
		"username": opts.Username,
		"user_id":  ownerID,
	})
}

// DeleteProfile removes a profile fixture and its username claim item.
func DeleteProfile(ctx context.Context, ownerID, username string) error {
	profileErr := NewDynamoTable(ctx, support.ProfileTableName()).
		DeleteItem(ctx, map[string]string{"user_id": ownerID})
	claimErr := NewDynamoTable(ctx, support.ProfileUsernameTableName()).
		DeleteItem(ctx, map[string]string{"username": username})

	return errors.Join(profileErr, claimErr)
}
