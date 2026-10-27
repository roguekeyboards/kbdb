package keyboards_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// keyboardWithParts decodes the parts of a keyboard response.
type keyboardWithParts struct {
	Plates []struct {
		ID        string   `json:"id"`
		Material  string   `json:"material"`
		Color     *string  `json:"color"`
		Thickness *float64 `json:"thickness"`
		Purchase  *struct {
			Vendor      *string  `json:"vendor"`
			Price       *float64 `json:"price"`
			OrderStatus *string  `json:"order_status"`
		} `json:"purchase"`
	} `json:"plates"`
	PCBs []struct {
		ID       string  `json:"id"`
		Firmware *string `json:"firmware"`
	} `json:"pcbs"`
	TotalCost *float64 `json:"total_cost"`
}

// approvedSize/approvedLayout/approvedCaseMaterial/approvedVendor are real
// values from internal/lookup/data/. approvedLayout is only valid for
// approvedSize, not approvedOtherSize - approvedOtherSize lets a spec send
// a size that passes the plain keyboard_size check but is wrong for
// approvedLayout, isolating the layout/size cross-check.
const (
	approvedSize             = "60%"
	approvedOtherSize        = "40%"
	approvedLayout           = "WK"
	approvedCaseMaterial     = "Aluminum"
	approvedVendor           = "Amazon"
	approvedImageContentType = "image/webp"
)

func TestKeyboards(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "REST Keyboards Suite")
}
