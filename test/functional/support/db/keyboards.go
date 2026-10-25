package db

import (
	"context"
	"fmt"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// The seeded keyboard's one plate and one PCB. The plate has its own
// purchase price, so the keyboard's total_cost is 329.99 + 40.
const (
	SeededPlateID = "seeded-plate"
	SeededPCBID   = "seeded-pcb"
)

// Plates and PCBs are maps keyed by part id, each entry carrying a seq
// ordering key (see repository.KeyboardPlate).
var (
	seededPlates = map[string]any{SeededPlateID: map[string]any{
		"id":       SeededPlateID,
		"material": "FR4",
		"color":    "Raw",
		"purchase": map[string]any{"vendor": "Amazon", "price": 40},
		"seq":      0,
	}}
	seededPCBs = map[string]any{SeededPCBID: map[string]any{
		"id":       SeededPCBID,
		"firmware": "QMK/VIA",
		"purchase": map[string]any{},
		"seq":      0,
	}}
)

// SeedKeyboard PutItems a keyboard directly into DynamoDB, bypassing the
// API - for specs that need keyboard fixture data in place before
// exercising a different route. images is seeded as an empty map, matching
// what Create writes - AddImage/DeleteImage address images.<id> in place,
// and DynamoDB rejects a nested-path write when the parent map is absent.
//
// The nested design/plates/pcbs/purchase groups are populated so reads exercise
// their real DynamoDB round trip. Seeding only the top-level fields would
// leave a dynamodbav tag mismatch on a nested group invisible to every
// spec, since the mappers' own tests construct Go structs directly and
// never touch DynamoDB.
func SeedKeyboard(ctx context.Context, ownerID, id, visibility string) error {
	table := NewDynamoTable(ctx, support.KeyboardTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "Keychron",
		"name":       "Q1",
		"size":       "60%",
		"visibility": visibility,
		"images":     map[string]any{},
		"design": map[string]any{
			"top_case": map[string]any{"material": "Aluminum", "color": "Black"},
		},
		"plates": seededPlates,
		"pcbs":   seededPCBs,
		"purchase": map[string]any{
			"vendor":       "Amazon",
			"price":        329.99,
			"order_status": "Delivered",
		},
	})
}

// SeedKeyboardWithImage is [SeedKeyboard] plus a single Images entry with
// no S3 object behind it, so the keyboard gets an image URL that can't be
// fetched. Images is a map keyed by image id, each entry carrying a seq
// ordering key (see repository.KeyboardImageEntry).
func SeedKeyboardWithImage(ctx context.Context, ownerID, id, imageID, visibility string) error {
	table := NewDynamoTable(ctx, support.KeyboardTableName())
	return table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"brand":      "Keychron",
		"name":       "Q1",
		"size":       "60%",
		"visibility": visibility,
		"design": map[string]any{
			"top_case": map[string]any{"material": "Aluminum", "color": "Black"},
		},
		"plates": seededPlates,
		"pcbs":   seededPCBs,
		"purchase": map[string]any{
			"vendor":       "Amazon",
			"price":        329.99,
			"order_status": "Delivered",
		},
		"images": map[string]any{
			imageID: map[string]any{
				"path": fmt.Sprintf("keyboards/%s/%s/images/%s", ownerID, id, imageID),
				"seq":  0,
			},
		},
	})
}

// DeleteKeyboard removes a keyboard seeded by SeedKeyboard, or one created
// via the API during a spec.
func DeleteKeyboard(ctx context.Context, ownerID, id string) error {
	table := NewDynamoTable(ctx, support.KeyboardTableName())
	return table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
}
