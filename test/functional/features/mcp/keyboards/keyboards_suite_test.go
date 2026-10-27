package keyboards_test

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKeyboards(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MCP Keyboards Suite")
}

// approvedSize/approvedLayout/approvedCaseMaterial/approvedVendor are real
// values from internal/lookup/data/. approvedLayout is valid only for
// approvedSize, so approvedOtherSize lets a spec send a size that passes
// the plain keyboard_size check but is wrong for the layout, isolating the
// cross-field rule from the plain membership check.
const (
	approvedSize             = "60%"
	approvedOtherSize        = "40%"
	approvedLayout           = "WK"
	approvedCaseMaterial     = "Aluminum"
	approvedVendor           = "Amazon"
	approvedImageContentType = "image/webp"
)

type getOutput struct {
	Keyboard struct {
		ID         string  `json:"id"`
		Brand      string  `json:"brand"`
		Name       string  `json:"name"`
		Size       *string `json:"size"`
		Visibility *string `json:"visibility"`
		HasImages  bool    `json:"has_images"`
		Design     *struct {
			TopCase *struct {
				Material *string `json:"material"`
				Color    *string `json:"color"`
			} `json:"top_case"`
			BottomCase *struct{} `json:"bottom_case"`
		} `json:"design"`
		Plates []struct {
			ID       string `json:"id"`
			Material string `json:"material"`
			Purchase *struct {
				Price *float64 `json:"price"`
			} `json:"purchase"`
		} `json:"plates"`
		PCBs []struct {
			ID       string  `json:"id"`
			Firmware *string `json:"firmware"`
		} `json:"pcbs"`
		TotalCost *float64 `json:"total_cost"`
		Purchase  *struct {
			Vendor      *string  `json:"vendor"`
			Price       *float64 `json:"price"`
			Currency    *string  `json:"currency"`
			OrderStatus *string  `json:"order_status"`
		} `json:"purchase"`
	} `json:"keyboard"`
}

func decodeGetOutput(result *sdkmcp.CallToolResult) getOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out getOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

type listKeyboard struct {
	ID          string   `json:"id"`
	Brand       string   `json:"brand"`
	Name        string   `json:"name"`
	OrderStatus *string  `json:"order_status"`
	TotalCost   *float64 `json:"total_cost"`
	Visibility  *string  `json:"visibility"`
}

type listOutput struct {
	Keyboards  []listKeyboard `json:"keyboards"`
	NextCursor string         `json:"next_cursor"`
}

func decodeListOutput(result *sdkmcp.CallToolResult) listOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out listOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

func idsOf(out listOutput) []string {
	ids := make([]string, 0, len(out.Keyboards))
	for _, kb := range out.Keyboards {
		ids = append(ids, kb.ID)
	}

	return ids
}

func seededBy(out listOutput, id string) *listKeyboard {
	for i := range out.Keyboards {
		if out.Keyboards[i].ID == id {
			return &out.Keyboards[i]
		}
	}

	return nil
}

type addImageOutput struct {
	ImageID   string `json:"image_id"`
	UploadURL string `json:"upload_url"`
}

func decodeAddImageOutput(result *sdkmcp.CallToolResult) addImageOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out addImageOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

type listImagesOutput struct {
	Images []struct {
		ImageID string `json:"image_id"`
	} `json:"images"`
}

func decodeListImagesOutput(result *sdkmcp.CallToolResult) listImagesOutput {
	GinkgoHelper()

	raw, err := json.Marshal(result.StructuredContent)
	Expect(err).NotTo(HaveOccurred())

	var out listImagesOutput
	Expect(json.Unmarshal(raw, &out)).To(Succeed())

	return out
}

func imageIDsOf(out listImagesOutput) []string {
	ids := make([]string, 0, len(out.Images))
	for _, img := range out.Images {
		ids = append(ids, img.ImageID)
	}

	return ids
}
