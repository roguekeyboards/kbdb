package keycapsets_test

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKeycapSets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MCP Keycap Sets Suite")
}

// approvedProfile/approvedMaterial/approvedImageContentType/approvedVendor/
// approvedOrderStatus are real values from internal/lookup/data/, matching
// what the REST keycap-sets suite and the MCP keyboards suite already use
// for the same lookup categories.
const (
	approvedProfile          = "Cherry/CYL"
	approvedMaterial         = "DyeSub PBT"
	approvedImageContentType = "image/webp"
	approvedVendor           = "Amazon"
	approvedOrderStatus      = "Ordered"
)

type keycapKitPurchase struct {
	Vendor      *string  `json:"vendor"`
	Price       *float64 `json:"price"`
	OrderStatus *string  `json:"order_status"`
}

type keycapKit struct {
	KitID    string             `json:"kit_id"`
	Name     string             `json:"name"`
	HasImage bool               `json:"has_image"`
	Purchase *keycapKitPurchase `json:"purchase"`
}

type keycapSet struct {
	ID           string      `json:"id"`
	Brand        string      `json:"brand"`
	Name         string      `json:"name"`
	Profile      *string     `json:"profile"`
	Material     *string     `json:"material"`
	Notes        *string     `json:"notes"`
	Visibility   string      `json:"visibility"`
	Kits         []keycapKit `json:"kits"`
	PrimaryKitID *string     `json:"primary_kit_id"`
	TotalCost    *float64    `json:"total_cost"`
	Currency     *string     `json:"currency"`
}

type getOutput struct {
	KeycapSet keycapSet `json:"keycap_set"`
}

func decodeKeycapSetOutput(result *sdkmcp.CallToolResult) getOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out getOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

type kitOutput struct {
	KeycapKit keycapKit `json:"keycap_kit"`
}

func decodeKeycapKitOutput(result *sdkmcp.CallToolResult) kitOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out kitOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

type listedKeycapSet struct {
	ID                 string   `json:"id"`
	Brand              string   `json:"brand"`
	Name               string   `json:"name"`
	Profile            *string  `json:"profile"`
	PrimaryKitID       *string  `json:"primary_kit_id"`
	PrimaryKitHasImage bool     `json:"primary_kit_has_image"`
	TotalCost          *float64 `json:"total_cost"`
}

type listOutput struct {
	KeycapSets []listedKeycapSet `json:"keycap_sets"`
	NextCursor string            `json:"next_cursor"`
}

func decodeListOutput(result *sdkmcp.CallToolResult) listOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out listOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

func decodeUploadURL(result *sdkmcp.CallToolResult) string {
	GinkgoHelper()

	var out struct {
		UploadURL string `json:"upload_url"`
	}
	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out.UploadURL
}

func idsOf(out listOutput) []string {
	ids := make([]string, 0, len(out.KeycapSets))
	for _, ks := range out.KeycapSets {
		ids = append(ids, ks.ID)
	}

	return ids
}

func seededBy(out listOutput, id string) *listedKeycapSet {
	for i := range out.KeycapSets {
		if out.KeycapSets[i].ID == id {
			return &out.KeycapSets[i]
		}
	}

	return nil
}
