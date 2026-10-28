package keycapsets_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// approvedProfile/approvedMaterial/approvedImageContentType are real values
// from internal/lookup/data/.
const (
	approvedProfile          = "Cherry/CYL"
	approvedMaterial         = "DyeSub PBT"
	approvedImageContentType = "image/webp"
)

func TestKeycapSets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "REST Keycap Sets Suite")
}
