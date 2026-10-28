package builds_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// approvedStabilizer/approvedStabilizerMountType/approvedCaseMountType/
// approvedDurometer are real values from internal/lookup/data/.
const (
	approvedStabilizer       = "Durock v3"
	approvedStabilizerMount  = "Screw-in"
	approvedCaseMountType    = "Gasket Mount"
	approvedDurometer        = "70A"
	approvedImageContentType = "image/webp"
)

func TestBuilds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "REST Builds Suite")
}
