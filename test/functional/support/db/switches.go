package db

import (
	"context"
	"fmt"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// SeedSwitch PutItems a switch directly into DynamoDB, bypassing the API -
// for specs that need switch fixture data in place before exercising a
// different route. version is set to 0, matching what Create would have
// set it to - Update/SetImagePath/ClearImagePath condition their write on
// version via a CAS loop, which fails on a fixture item missing the
// attribute entirely (attribute_not_exists != 0), not just a genuine
// version mismatch.
func SeedSwitch(ctx context.Context, ownerID, id, visibility string) error {
	table := NewDynamoTable(ctx, support.SwitchTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "Gateron",
		"name":       "Yellow",
		"type":       "Linear",
		"visibility": visibility,
		"version":    0,
		"purchase": map[string]any{
			"vendor":       "NovelKeys",
			"price":        0.35,
			"order_status": "Delivered",
		},
	})
}

// SeedSwitchWithImage is [SeedSwitch] plus an ImagePath, whose path doesn't
// need a real S3 object behind it - presigning a GET URL doesn't check the
// object exists, only specs that fetch the URL's content would need that.
func SeedSwitchWithImage(ctx context.Context, ownerID, id, visibility string) error {
	table := NewDynamoTable(ctx, support.SwitchTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "Gateron",
		"name":       "Yellow",
		"type":       "Linear",
		"visibility": visibility,
		"version":    0,
		"purchase": map[string]any{
			"vendor": "NovelKeys",
			"price":  0.35,
		},
		"image_path": fmt.Sprintf("switches/%s/%s/image/up1", ownerID, id),
	})
}

// DeleteSwitch removes a switch seeded by SeedSwitch, or one created via the
// API during a spec.
func DeleteSwitch(ctx context.Context, ownerID, id string) error {
	table := NewDynamoTable(ctx, support.SwitchTableName())
	return table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
}
