package switches_test

import (
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

// GET is one of the OptionalAuth routes, which accept anonymous callers -
// the case worth pinning is that a token which fails to verify is rejected
// rather than treated as no token at all.
var _ = Describe("Sending a malformed bearer token to an anonymous-capable route", func() {
	var (
		resp     *http.Response
		client   *api.SwitchesClient
		ownerID  string
		switchID string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewSwitchesClient()

		var err error
		_, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a public switch and a malformed bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			switchID = "functional-test-switch-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})

		When("getting the switch", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, switchID, "not-a-valid-jwt")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 401, not silently falling back to anonymous", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})
})
