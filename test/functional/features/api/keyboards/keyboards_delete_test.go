package keyboards_test

import (
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Deleting a keyboard", func() {
	var (
		resp       *http.Response
		client     *api.KeyboardsClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeyboardsClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a private keyboard owned by the caller", func() {
		var keyboardID string
		var deleted bool

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "delete-keyboard-" + uuid.NewString()
			deleted = false
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			if !deleted {
				Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
			}
		})

		Context("given the caller is the owner", func() {
			When("deleting the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keyboardID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusNoContent {
						deleted = true
					}
				})

				It("returns 204 and the keyboard is gone", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer being gettable")
					getResp, err := client.Get(ctx, ownerID, keyboardID, ownerToken)
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

			When("deleting the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keyboardID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keyboardID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the keyboard does not exist", func() {
		Context("given the caller is the owner", func() {
			When("deleting the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-keyboard-"+uuid.NewString(), ownerToken)
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

			When("deleting the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-keyboard-"+uuid.NewString(), token)
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
