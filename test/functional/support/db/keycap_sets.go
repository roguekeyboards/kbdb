package db

import (
	"context"
	"strconv"

	"github.com/roguekeyboards/kbdb/test/functional/support"
)

// SeedKeycapSet PutItems a keycap set directly into DynamoDB, bypassing the
// API - for specs that need keycap set fixture data in place before
// exercising a different route.
func SeedKeycapSet(ctx context.Context, ownerID, id, visibility string) error {
	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "GMK",
		"name":       "Laser",
		"kits":       map[string]any{},
		"visibility": visibility,
	})
}

// DeleteKeycapSet removes a keycap set seeded by SeedKeycapSet, or one
// created via the API during a spec.
func DeleteKeycapSet(ctx context.Context, ownerID, id string) error {
	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
}

// SeedKeycapSetWithKit is SeedKeycapSet, but includes a single kit with the
// given kitID in its Kits, so DeleteKit specs have something to delete.
func SeedKeycapSetWithKit(ctx context.Context, ownerID, id, kitID, visibility string) error {
	return SeedKeycapSetWithKits(ctx, ownerID, id, []string{kitID}, visibility)
}

// SeedKeycapSetWithKits is SeedKeycapSet, but includes one kit per given
// kitID in its Kits, so whole-set DeleteKeycapSet specs can exercise
// multiple kits referenced by different builds.
func SeedKeycapSetWithKits(ctx context.Context, ownerID, id string, kitIDs []string, visibility string) error {
	kits := make(map[string]any, len(kitIDs))
	for _, kitID := range kitIDs {
		kits[kitID] = map[string]any{"kit_id": kitID, "name": "Base", "purchase": map[string]any{
			"vendor": "MechMarket",
			"price":  85.0,
		}}
	}

	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "GMK",
		"name":       "Laser",
		"kits":       kits,
		"visibility": visibility,
	})
}

// SeedKeycapSetWithKitOrderStatuses is SeedKeycapSet, but includes one kit
// per given order status, so specs can exercise the derived
// order_status bubble-up rule across multiple kits.
func SeedKeycapSetWithKitOrderStatuses(ctx context.Context, ownerID, id string, orderStatuses []string, visibility string) error {
	kits := make(map[string]any, len(orderStatuses))
	for i, status := range orderStatuses {
		kitID := "kit-" + strconv.Itoa(i)
		kits[kitID] = map[string]any{"kit_id": kitID, "name": "Base", "purchase": map[string]any{
			"order_status": status,
		}}
	}

	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "GMK",
		"name":       "Laser",
		"kits":       kits,
		"visibility": visibility,
	})
}

// SeedKeycapSetWithPrimaryKit is SeedKeycapSetWithKit, but also sets
// primary_kit_id to kitID directly, skipping the create/update-kit round
// trip a spec would otherwise need to designate a primary.
func SeedKeycapSetWithPrimaryKit(ctx context.Context, ownerID, id, kitID, visibility string) error {
	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id": ownerID,
		"id":      id,
		"brand":   "GMK",
		"name":    "Laser",
		"kits": map[string]any{
			kitID: map[string]any{"kit_id": kitID, "name": "Base", "purchase": map[string]any{
				"vendor": "MechMarket",
				"price":  85.0,
			}},
		},
		"primary_kit_id": kitID,
		"visibility":     visibility,
	})
}

// SeedKeycapSetWithDanglingPrimaryKit is SeedKeycapSet, but sets
// primary_kit_id to a kit id absent from Kits - simulates the primary kit
// having been deleted after being designated.
func SeedKeycapSetWithDanglingPrimaryKit(ctx context.Context, ownerID, id, visibility string) error {
	table := NewDynamoTable(ctx, support.KeycapSetTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":        ownerID,
		"id":             id,
		"brand":          "GMK",
		"name":           "Laser",
		"primary_kit_id": "no-longer-a-kit",
		"visibility":     visibility,
	})
}
