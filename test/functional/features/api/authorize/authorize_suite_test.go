package authorize_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/browser"
)

var chromium *browser.Browser

func TestAuthorize(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GET /authorize Suite")
}

var _ = BeforeSuite(func() {
	var err error
	chromium, err = browser.Start()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(chromium.Stop)
})
