package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Dumps taken before keyboards had plate and PCB lists (#483/#484) hold
// design.plates as material strings, a single pcb object, and a build plate
// that's a material string. The upgrade functions rewrite those into the
// current shape so restore and verify read every dump the same way.
//
// A legacy part has no id, so it gets a stand-in the dump can be mapped
// through: a plate's is its material, suffixed "#2", "#3"... for repeats,
// and the PCB's is legacyPCBID. A legacy build plate names its keyboard's
// first plate of that material, so it maps to the unsuffixed id.
const legacyPCBID = "pcb"

func legacyPlateID(material string, nth int) string {
	if nth == 1 {
		return material
	}
	return fmt.Sprintf("%s#%d", material, nth)
}

// upgradeKeyboardJSON returns raw in the current keyboard shape.
func upgradeKeyboardJSON(raw []byte) ([]byte, error) {
	var kb map[string]any
	if err := json.Unmarshal(raw, &kb); err != nil {
		return nil, err
	}

	changed := false
	if design, ok := kb["design"].(map[string]any); ok {
		if plates, ok := design["plates"].([]any); ok {
			seen := map[string]int{}
			upgraded := make([]any, 0, len(plates))
			for _, p := range plates {
				material, ok := p.(string)
				if !ok {
					return nil, fmt.Errorf("legacy design.plates entry %v is not a string", p)
				}
				seen[material]++
				upgraded = append(upgraded, map[string]any{"id": legacyPlateID(material, seen[material]), "material": material})
			}
			delete(design, "plates")
			if len(design) == 0 {
				delete(kb, "design")
			}
			if len(upgraded) > 0 {
				kb["plates"] = upgraded
			}
			changed = true
		}
	}
	if pcb, ok := kb["pcb"].(map[string]any); ok {
		delete(kb, "pcb")
		if len(pcb) > 0 {
			pcb["id"] = legacyPCBID
			kb["pcbs"] = []any{pcb}
		}
		changed = true
	}

	if !changed {
		return raw, nil
	}
	return json.Marshal(kb)
}

// upgradeBuildJSON returns raw in the current build shape.
func upgradeBuildJSON(raw []byte) ([]byte, error) {
	var b map[string]any
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}

	material, ok := b["plate"].(string)
	if !ok {
		return raw, nil
	}
	b["plate"] = map[string]any{"id": legacyPlateID(material, 1), "material": material}
	return json.Marshal(b)
}

// readUpgradedJSON reads a dumped item, upgrades it, and parses it into v.
func readUpgradedJSON(path string, upgrade func([]byte) ([]byte, error), v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // path is inside the dump dir.
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	data, err = upgrade(data)
	if err != nil {
		return fmt.Errorf("upgrading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}
