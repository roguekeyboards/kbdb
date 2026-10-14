package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/rogueserenity/kbdb/test/functional/support"
)

// keyboardRefMarkerSortKey mirrors the marker item sort key format
// [github.com/rogueserenity/kbdb/internal/repository/dynamo] writes for a
// keyboard reference, so seeded builds are discoverable via
// [github.com/rogueserenity/kbdb/internal/repository.BuildRepository.FindBuildsReferencingKeyboard]
// without needing a real Create call.
func keyboardRefMarkerSortKey(keyboardID, buildID string) string {
	return fmt.Sprintf("zREF#keyboard#%s#%s", keyboardID, buildID)
}

// switchRefMarkerSortKey mirrors the marker item sort key format
// [github.com/rogueserenity/kbdb/internal/repository/dynamo] writes for a
// switch reference, so seeded builds are discoverable via
// [github.com/rogueserenity/kbdb/internal/repository.BuildRepository.FindBuildsReferencingSwitch]
// without needing a real Create call.
func switchRefMarkerSortKey(switchID, buildID string) string {
	return fmt.Sprintf("zREF#switch#%s#%s", switchID, buildID)
}

// keycapKitRefID mirrors the composite ref_id
// [github.com/rogueserenity/kbdb/internal/repository/dynamo] derives for a
// keycap kit reference (KeycapSetID#KitID).
func keycapKitRefID(keycapSetID, kitID string) string {
	return fmt.Sprintf("%s#%s", keycapSetID, kitID)
}

// keycapKitRefMarkerSortKey mirrors the marker item sort key format
// [github.com/rogueserenity/kbdb/internal/repository/dynamo] writes for a
// keycap kit reference, so seeded builds are discoverable via
// [github.com/rogueserenity/kbdb/internal/repository.BuildRepository.FindBuildsReferencingKeycapKit]
// without needing a real Create call.
func keycapKitRefMarkerSortKey(keycapSetID, kitID, buildID string) string {
	return fmt.Sprintf("zREF#keycap_kit#%s#%s", keycapKitRefID(keycapSetID, kitID), buildID)
}

// SeedBuild writes directly to the table, bypassing buildrefs' existence
// check - keyboardID isn't validated against a real keyboard.
func SeedBuild(ctx context.Context, ownerID, id, keyboardID, visibility string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"keyboard":   keyboardID,
		"visibility": visibility,
		"images":     map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	})
}

// SeedBuildWithStabs is SeedBuild, but includes stabs with a price, for
// specs asserting on Stabs.Price visibility.
func SeedBuildWithStabs(ctx context.Context, ownerID, id, keyboardID, visibility string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"keyboard":   keyboardID,
		"visibility": visibility,
		"images":     map[string]any{},
		"stabs": map[string]any{
			"name":       "Durock v3",
			"mount_type": "Screw-in",
			"price":      12.5,
		},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	})
}

// SeedBuildWithParts is SeedBuild, but the build uses keyboardID's seeded
// plate and PCB (SeededPlateID/SeededPCBID).
func SeedBuildWithParts(ctx context.Context, ownerID, id, keyboardID, visibility string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"keyboard":   keyboardID,
		"plate":      SeededPlateID,
		"pcb":        SeededPCBID,
		"visibility": visibility,
		"images":     map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	})
}

// DeleteBuild removes the build with id, and its keyboard reverse-reference
// marker, from the table. Both deletes are attempted even if the first
// fails, so a transient failure on one doesn't orphan the other in the
// shared, long-lived table CI deploys per PR (see .github/workflows/ci.yml
// - that stack persists across every push until the PR closes, so leftover
// rows aren't cleaned up by a fresh-container teardown the way a local
// floci run's would be).
func DeleteBuild(ctx context.Context, ownerID, id, keyboardID string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	markerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keyboardRefMarkerSortKey(keyboardID, id),
	})

	return errors.Join(buildErr, markerErr)
}

// SeedBuildWithSwitch writes directly to the table, bypassing buildrefs'
// existence check - switchID isn't validated against a real switch.
func SeedBuildWithSwitch(ctx context.Context, ownerID, id, switchID, visibility string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"switches":   []map[string]any{{"switch": switchID, "count": 1}},
		"visibility": visibility,
		"images":     map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        switchRefMarkerSortKey(switchID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "switch",
		"ref_id":    switchID,
		"build_id":  id,
	})
}

// SeedBuildWithSwitchAndKeyboard is SeedBuildWithSwitch, but also sets the
// build's keyboard field and writes the matching keyboard reverse-reference
// marker - for specs that GET the build back and need a real (resolvable)
// keyboard reference, unlike SeedBuildWithSwitch's builds, which have none.
func SeedBuildWithSwitchAndKeyboard(ctx context.Context, ownerID, id, keyboardID, switchID, visibility string) error {
	if err := SeedBuildWithSwitch(ctx, ownerID, id, switchID, visibility); err != nil {
		return err
	}

	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"keyboard":   keyboardID,
		"switches":   []map[string]any{{"switch": switchID, "count": 1}},
		"visibility": visibility,
		"images":     map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	})
}

// DeleteBuildWithSwitchAndKeyboard removes everything SeedBuildWithSwitchAndKeyboard wrote.
func DeleteBuildWithSwitchAndKeyboard(ctx context.Context, ownerID, id, keyboardID, switchID string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	switchMarkerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      switchRefMarkerSortKey(switchID, id),
	})
	keyboardMarkerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keyboardRefMarkerSortKey(keyboardID, id),
	})

	return errors.Join(buildErr, switchMarkerErr, keyboardMarkerErr)
}

// SeedBuildWithSwitchesAndKeyboard is SeedBuildWithSwitchAndKeyboard, but
// takes one switches[] entry per given switchID (each with count 1) - for
// specs that need a build referencing more than one switch, e.g. to check
// that resolving one entry's reference doesn't affect another's.
func SeedBuildWithSwitchesAndKeyboard(ctx context.Context, ownerID, id, keyboardID string, switchIDs []string, visibility string) error {
	switches := make([]map[string]any, len(switchIDs))
	for i, switchID := range switchIDs {
		switches[i] = map[string]any{"switch": switchID, "count": 1}
	}

	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":    ownerID,
		"id":         id,
		"keyboard":   keyboardID,
		"switches":   switches,
		"visibility": visibility,
		"images":     map[string]any{},
	}); err != nil {
		return err
	}

	if err := table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	}); err != nil {
		return err
	}

	for _, switchID := range switchIDs {
		if err := table.PutItem(ctx, map[string]any{
			"user_id":   ownerID,
			"id":        switchRefMarkerSortKey(switchID, id),
			"item_type": "build_ref_marker",
			"ref_type":  "switch",
			"ref_id":    switchID,
			"build_id":  id,
		}); err != nil {
			return err
		}
	}

	return nil
}

// DeleteBuildWithSwitchesAndKeyboard removes everything
// SeedBuildWithSwitchesAndKeyboard wrote.
func DeleteBuildWithSwitchesAndKeyboard(ctx context.Context, ownerID, id, keyboardID string, switchIDs []string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	keyboardMarkerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keyboardRefMarkerSortKey(keyboardID, id),
	})

	var switchMarkerErrs []error
	for _, switchID := range switchIDs {
		switchMarkerErrs = append(switchMarkerErrs, table.DeleteItem(ctx, map[string]string{
			"user_id": ownerID,
			"id":      switchRefMarkerSortKey(switchID, id),
		}))
	}

	return errors.Join(append([]error{buildErr, keyboardMarkerErr}, switchMarkerErrs...)...)
}

// DeleteBuildWithSwitch removes the build with id, and its switch
// reverse-reference marker, from the table. Both deletes are attempted even
// if the first fails - see DeleteBuild's doc comment for why.
func DeleteBuildWithSwitch(ctx context.Context, ownerID, id, switchID string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	markerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      switchRefMarkerSortKey(switchID, id),
	})

	return errors.Join(buildErr, markerErr)
}

// SeedBuildWithKeycapKit writes directly to the table, bypassing buildrefs'
// existence check - keycapSetID/kitID aren't validated against a real
// keycap set/kit.
func SeedBuildWithKeycapKit(ctx context.Context, ownerID, id, keycapSetID, kitID, visibility string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":     ownerID,
		"id":          id,
		"keycap_kits": []map[string]any{{"keycap_set": keycapSetID, "kit": kitID}},
		"visibility":  visibility,
		"images":      map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keycapKitRefMarkerSortKey(keycapSetID, kitID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keycap_kit",
		"ref_id":    keycapKitRefID(keycapSetID, kitID),
		"build_id":  id,
	})
}

// SeedBuildWithKeycapKitAndKeyboard is SeedBuildWithKeycapKit, but also sets
// the build's keyboard field and writes the matching keyboard
// reverse-reference marker - for specs that GET the build back and need a
// real (resolvable) keyboard reference, unlike SeedBuildWithKeycapKit's
// builds, which have none.
func SeedBuildWithKeycapKitAndKeyboard(ctx context.Context, ownerID, id, keyboardID, keycapSetID, kitID, visibility string) error {
	if err := SeedBuildWithKeycapKit(ctx, ownerID, id, keycapSetID, kitID, visibility); err != nil {
		return err
	}

	table := NewDynamoTable(ctx, support.BuildTableName())
	if err := table.PutItem(ctx, map[string]any{
		"user_id":     ownerID,
		"id":          id,
		"keyboard":    keyboardID,
		"keycap_kits": []map[string]any{{"keycap_set": keycapSetID, "kit": kitID}},
		"visibility":  visibility,
		"images":      map[string]any{},
	}); err != nil {
		return err
	}

	return table.PutItem(ctx, map[string]any{
		"user_id":   ownerID,
		"id":        keyboardRefMarkerSortKey(keyboardID, id),
		"item_type": "build_ref_marker",
		"ref_type":  "keyboard",
		"ref_id":    keyboardID,
		"build_id":  id,
	})
}

// DeleteBuildWithKeycapKitAndKeyboard removes everything SeedBuildWithKeycapKitAndKeyboard wrote.
func DeleteBuildWithKeycapKitAndKeyboard(ctx context.Context, ownerID, id, keyboardID, keycapSetID, kitID string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	kitMarkerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keycapKitRefMarkerSortKey(keycapSetID, kitID, id),
	})
	keyboardMarkerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keyboardRefMarkerSortKey(keyboardID, id),
	})

	return errors.Join(buildErr, kitMarkerErr, keyboardMarkerErr)
}

// DeleteBuildWithKeycapKit removes the build with id, and its keycap kit
// reverse-reference marker, from the table. Both deletes are attempted even
// if the first fails - see DeleteBuild's doc comment for why.
func DeleteBuildWithKeycapKit(ctx context.Context, ownerID, id, keycapSetID, kitID string) error {
	table := NewDynamoTable(ctx, support.BuildTableName())

	buildErr := table.DeleteItem(ctx, map[string]string{"user_id": ownerID, "id": id})
	markerErr := table.DeleteItem(ctx, map[string]string{
		"user_id": ownerID,
		"id":      keycapKitRefMarkerSortKey(keycapSetID, kitID, id),
	})

	return errors.Join(buildErr, markerErr)
}
