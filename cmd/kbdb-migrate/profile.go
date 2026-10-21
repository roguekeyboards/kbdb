package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"

	"github.com/rogueserenity/kbdb/internal/handlers/api"
)

// profileDir is where a dump keeps the profile: profile.json plus the avatar
// as a single-slot image named "avatar".
const profileDir = "profile"

// profilePath is the caller's own profile route.
func (c *apiClient) profilePath() string {
	return "/v1/profile/" + c.subject
}

// avatarURL reads avatar.url from a profile body, "" if there is none (no
// avatar, or a rejected one).
func avatarURL(body []byte) string {
	var p api.Profile
	if err := json.Unmarshal(body, &p); err != nil || p.Avatar == nil || p.Avatar.Url == nil {
		return ""
	}
	return *p.Avatar.Url
}

// isNotFound reports whether err is an API 404.
func isNotFound(err error) bool {
	var apiErr *apiError
	return asAPIError(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// dumpProfile writes the subject's profile and avatar under outDir/profile.
// Returns whether there was a profile, and how many images it wrote.
func dumpProfile(ctx context.Context, client *apiClient, outDir string) (bool, int, error) {
	fmt.Println("fetching profile...")
	body, err := client.getRaw(ctx, client.profilePath())
	if isNotFound(err) {
		fmt.Println("  no profile")
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("fetching profile: %w", err)
	}

	dir := filepath.Join(outDir, profileDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, 0, fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), body, 0o600); err != nil {
		return false, 0, fmt.Errorf("writing profile.json: %w", err)
	}

	url := avatarURL(body)
	if url == "" {
		fmt.Println("  profile (no avatar)")
		return true, 0, nil
	}
	entry, err := downloadOne(ctx, dir, "avatar", imageRef{url: url})
	if err != nil {
		return false, 0, fmt.Errorf("profile: %w", err)
	}
	if err := writeJSONFile(filepath.Join(dir, "avatar.manifest.json"), entry); err != nil {
		return false, 0, err
	}
	fmt.Println("  profile (1 avatar)")
	return true, 1, nil
}

// restoreProfile creates the target's profile from the dump, or, when the
// target already has one, replaces its fields but keeps its username. It runs
// before anything else is restored, so a dumped username that another account
// holds stops the restore before it writes anything.
func restoreProfile(ctx context.Context, client *apiClient, dumpDir string) error {
	dir := filepath.Join(dumpDir, profileDir)
	path := filepath.Join(dir, "profile.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var dumped api.Profile
	if err := readJSONFile(path, &dumped); err != nil {
		return err
	}
	input := api.ProfileInput{
		Bio:             dumped.Bio,
		DiscordUsername: dumped.DiscordUsername,
		Discoverable:    &dumped.Discoverable,
		Links:           dumped.Links,
		Preferences:     dumped.Preferences,
		Username:        dumped.Username,
	}

	var existing api.Profile
	err := client.doJSON(ctx, http.MethodGet, client.profilePath(), nil, &existing)
	switch {
	case isNotFound(err):
		var apiErr *apiError
		if err := client.doJSON(ctx, http.MethodPost, client.profilePath(), input, nil); err != nil {
			if asAPIError(err, &apiErr) && apiErr.Status == http.StatusConflict {
				return fmt.Errorf("creating profile %q (nothing has been restored; is the username another account's?): %w", input.Username, err)
			}
			return fmt.Errorf("creating profile %q: %w", input.Username, err)
		}
		fmt.Printf("profile %q created\n", input.Username)
	case err != nil:
		return fmt.Errorf("fetching the target's profile: %w", err)
	default:
		input.Username = existing.Username
		if err := client.doJSON(ctx, http.MethodPut, client.profilePath(), input, nil); err != nil {
			return fmt.Errorf("updating profile %q: %w", input.Username, err)
		}
		if existing.Username != dumped.Username {
			fmt.Printf("profile %q updated (kept its username; the dump's is %q)\n", existing.Username, dumped.Username)
		} else {
			fmt.Printf("profile %q updated\n", existing.Username)
		}
	}

	if err := restoreAvatar(ctx, client, dir); err != nil {
		return fmt.Errorf("profile avatar: %w", err)
	}
	return nil
}

// restoreAvatar uploads the dumped avatar unless the target's already matches
// it, so a re-run doesn't upload it again.
func restoreAvatar(ctx context.Context, client *apiClient, dir string) error {
	entry, data, found, err := loadSingleImageManifest(dir, "avatar")
	if err != nil || !found {
		return err
	}
	body, err := client.getRaw(ctx, client.profilePath())
	if err != nil {
		return fmt.Errorf("fetching profile to check its avatar: %w", err)
	}
	if url := avatarURL(body); url != "" {
		live, _, err := downloadImage(ctx, url)
		if err != nil {
			return fmt.Errorf("downloading the target's avatar: %w", err)
		}
		if sha256Hex(live) == entry.SHA256 {
			return nil
		}
	}
	if err := uploadSingleSlot(ctx, client, client.profilePath()+"/image", entry, data); err != nil {
		return err
	}
	body, err = client.getRaw(ctx, client.profilePath())
	if err != nil {
		return fmt.Errorf("re-fetching profile to verify avatar: %w", err)
	}
	return verifyURLHash(ctx, avatarURL(body), entry.SHA256)
}

// verifyProfile checks the target's profile against the dumped one. Usernames
// aren't compared, since restore keeps a target profile's own, and an avatar
// is checked only when the dump has one.
func verifyProfile(ctx context.Context, client *apiClient, dumpDir string) []verifyResult {
	dir := filepath.Join(dumpDir, profileDir)
	var dumped api.Profile
	if err := readJSONFile(filepath.Join(dir, "profile.json"), &dumped); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []verifyResult{{entity: "profile", newID: client.subject, status: statusMissing, detail: err.Error()}}
	}
	r := verifyResult{entity: "profile", oldID: dumped.UserId, newID: client.subject}

	liveBody, err := client.getRaw(ctx, client.profilePath())
	if err != nil {
		r.status, r.detail = statusMissing, err.Error()
		return []verifyResult{r}
	}
	var live api.Profile
	if err := json.Unmarshal(liveBody, &live); err != nil {
		r.status, r.detail = statusFieldMismatch, "live profile unparseable: "+err.Error()
		return []verifyResult{r}
	}
	for _, p := range []*api.Profile{&dumped, &live} {
		p.UserId, p.Username, p.Avatar = "", "", nil
	}
	if !reflect.DeepEqual(dumped, live) {
		r.status, r.detail = statusFieldMismatch, "fields differ from the dump"
		return []verifyResult{r}
	}
	if hasSingleImage(dir, "avatar") {
		if st, detail := verifySingleImage(ctx, dir, "avatar", avatarURL(liveBody)); st != statusOK {
			r.status, r.detail = st, detail
			return []verifyResult{r}
		}
	}
	r.status = statusOK
	return []verifyResult{r}
}
