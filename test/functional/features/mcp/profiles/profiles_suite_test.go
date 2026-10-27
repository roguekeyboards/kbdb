package profiles_test

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProfiles(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MCP Profiles Suite")
}

type getProfileOutput struct {
	Profile struct {
		Username        string  `json:"username"`
		UserID          string  `json:"user_id"`
		Discoverable    bool    `json:"discoverable"`
		DiscordUsername *string `json:"discord_username"`
		Bio             *string `json:"bio"`
		Links           []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
		HasAvatar   bool `json:"has_avatar"`
		Preferences *struct {
			Currency          string `json:"currency"`
			ShowPriceToMe     bool   `json:"show_price_to_me"`
			ShowPriceToOthers bool   `json:"show_price_to_others"`
		} `json:"preferences"`
	} `json:"profile"`
}

func decodeGetProfileOutput(result *sdkmcp.CallToolResult) getProfileOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out getProfileOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

type listProfilesOutput struct {
	Profiles []struct {
		Username        string  `json:"username"`
		UserID          string  `json:"user_id"`
		DiscordUsername *string `json:"discord_username"`
		HasAvatar       bool    `json:"has_avatar"`
		// Deliberately not in the summary shape - specs assert absent.
		Bio   *string `json:"bio"`
		Links []any   `json:"links"`
	} `json:"profiles"`
	NextCursor string `json:"next_cursor"`
}

func decodeListProfilesOutput(result *sdkmcp.CallToolResult) listProfilesOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out listProfilesOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

func listProfileUsernames(out listProfilesOutput) []string {
	names := make([]string, len(out.Profiles))
	for i, p := range out.Profiles {
		names[i] = p.Username
	}
	return names
}

// approvedImageContentType is a value seeded in the image_content_type
// lookup category.
const approvedImageContentType = "image/webp"

type setProfileImageOutput struct {
	UploadURL string `json:"upload_url"`
}

func decodeSetProfileImageOutput(result *sdkmcp.CallToolResult) setProfileImageOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out setProfileImageOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}
