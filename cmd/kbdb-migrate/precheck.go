package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rogueserenity/kbdb/internal/repository"
)

// checkImageLimits walks every image restore would upload and reports each
// one the API would reject - over the size cap, empty, or past an item's
// image count cap - before restore writes anything, rather than failing
// partway through. Sizes come from the files on disk, not the manifest, so a
// file shrunk in place after the dump is judged by what will be sent.
func checkImageLimits(dumpDir string) error {
	var problems []string

	for _, sub := range []string{"keyboards", "builds"} {
		ids, err := itemDirs(dumpDir, sub)
		if err != nil {
			return err
		}
		for _, id := range ids {
			imagesDir := filepath.Join(dumpDir, sub, id, "images")
			manifestPath := filepath.Join(imagesDir, "images.manifest.json")
			if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
				continue
			}
			var manifest []imageManifestEntry
			if err := readJSONFile(manifestPath, &manifest); err != nil {
				return err
			}
			if len(manifest) > repository.MaxImagesPerItem {
				problems = append(problems, fmt.Sprintf("%s/%s has %d images, over the %d-image cap",
					sub, id, len(manifest), repository.MaxImagesPerItem))
			}
			for _, entry := range manifest {
				problem, err := checkImageSize(filepath.Join(imagesDir, entry.Filename))
				if err != nil {
					return err
				}
				if problem != "" {
					problems = append(problems, problem)
				}
			}
		}
	}

	type singleSlot struct{ dir, name string }
	singleSlots := []singleSlot{{filepath.Join(dumpDir, profileDir), "avatar"}}
	switchIDs, err := itemDirs(dumpDir, "switches")
	if err != nil {
		return err
	}
	for _, id := range switchIDs {
		singleSlots = append(singleSlots, singleSlot{filepath.Join(dumpDir, "switches", id), "image"})
	}
	setIDs, err := itemDirs(dumpDir, "keycap-sets")
	if err != nil {
		return err
	}
	for _, setID := range setIDs {
		setDir := filepath.Join(dumpDir, "keycap-sets", setID)
		kitIDs, err := itemDirs(setDir, "kits")
		if err != nil {
			return err
		}
		for _, kitID := range kitIDs {
			singleSlots = append(singleSlots, singleSlot{filepath.Join(setDir, "kits", kitID), "image"})
		}
	}
	for _, slot := range singleSlots {
		manifestPath := filepath.Join(slot.dir, slot.name+".manifest.json")
		if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		var entry imageManifestEntry
		if err := readJSONFile(manifestPath, &entry); err != nil {
			return err
		}
		problem, err := checkImageSize(filepath.Join(slot.dir, entry.Filename))
		if err != nil {
			return err
		}
		if problem != "" {
			problems = append(problems, problem)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("dump has images the API would reject; fix them in the source environment and dump again:\n  %s",
			strings.Join(problems, "\n  "))
	}
	return nil
}

// checkImageSize returns a description of why path's size would be
// rejected, or "" if it's within the API's bounds.
func checkImageSize(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("checking %s: %w", path, err)
	}
	if info.Size() < 1 || info.Size() > repository.MaxImageSizeBytes {
		return fmt.Sprintf("%s is %d bytes, outside 1-%d", path, info.Size(), repository.MaxImageSizeBytes), nil
	}
	return "", nil
}
