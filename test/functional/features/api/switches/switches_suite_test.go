package switches_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// approvedType/approvedStem/approvedSpringMaterial/approvedVendor are real
// values from internal/lookup/data/.
const (
	approvedType             = "Linear"
	approvedStem             = "POM"
	approvedSpringMaterial   = "Stainless Steel"
	approvedVendor           = "Amazon"
	approvedImageContentType = "image/webp"
)

func TestSwitches(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "REST Switches Suite")
}
