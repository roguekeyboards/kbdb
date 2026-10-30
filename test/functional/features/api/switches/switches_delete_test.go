package switches_test

import (
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a switch", func() {
	var (
		resp       *http.Response
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewSwitchesClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a private switch owned by the caller", func() {
		var switchID string
		var deleted bool

		BeforeEach(func(ctx SpecContext) {
			switchID = "delete-switch-" + uuid.NewString()
			deleted = false
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			if !deleted {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			}
		})

		Context("given the caller is the owner", func() {
			When("deleting the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, switchID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusNoContent {
						deleted = true
					}
				})

				It("returns 204 and the switch is gone", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer being gettable")
					getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					Expect(getResp.StatusCode).To(Equal(http.StatusNotFound))
				})
			})
		})

		Context("given the caller is a different authenticated user", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("deleting the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, switchID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, switchID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the switch does not exist", func() {
		Context("given the caller is the owner", func() {
			When("deleting the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-switch-"+uuid.NewString(), ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204, idempotently", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
				})
			})
		})

		Context("given the caller is a different authenticated user", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("deleting the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-switch-"+uuid.NewString(), token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not the owner's idempotent 204", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})
})
