package keyboards_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Listing keyboards", func() {
	var (
		resp       *http.Response
		client     *api.KeyboardsClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeyboardsClient()

		// Derived from a freshly minted token, not a fixed fixture subject:
		// AuthToken mints a real subject from the oidc-testkit-signed token
		// rather than a fixed fixture subject string, so the owner used
		// to seed fixture data below must match whatever subject this
		// environment's token actually carries.
		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	itemIDs := func(r *http.Response) []string {
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&page)).To(Succeed())
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = item.ID
		}
		return ids
	}

	itemByID := func(r *http.Response, id string) map[string]any {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&page)).To(Succeed())
		for _, item := range page.Items {
			if item["id"] == id {
				return item
			}
		}
		return nil
	}

	purchaseOf := func(item map[string]any) map[string]any {
		purchase, _ := item["purchase"].(map[string]any)
		return purchase
	}

	Context("given the owner has keyboards at every visibility tier", func() {
		var publicID, authenticatedID, privateID string

		BeforeEach(func(ctx SpecContext) {
			// Fresh IDs per spec run, not fixed literals: AuthToken is a
			// shared, real emulator identity in CI (provisioning one per
			// spec isn't practical there), so specs sharing an identity
			// must namespace their own data to stay collision-proof under
			// concurrent/out-of-order runs.
			publicID = "public-keyboard-" + uuid.NewString()
			authenticatedID = "authenticated-keyboard-" + uuid.NewString()
			privateID = "private-keyboard-" + uuid.NewString()

			Expect(db.SeedKeyboard(ctx, ownerID, publicID, "public")).To(Succeed())
			Expect(db.SeedKeyboard(ctx, ownerID, authenticatedID, "authenticated")).To(Succeed())
			Expect(db.SeedKeyboard(ctx, ownerID, privateID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, publicID)).To(Succeed())
			Expect(db.DeleteKeyboard(ctx, ownerID, authenticatedID)).To(Succeed())
			Expect(db.DeleteKeyboard(ctx, ownerID, privateID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns keyboards at every visibility tier", func() {
					By("returning 200 OK")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					By("including all three seeded keyboards")
					Expect(itemIDs(resp)).To(ContainElements(publicID, authenticatedID, privateID))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, "", -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns only the public keyboard", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					ids := itemIDs(resp)
					Expect(ids).To(ContainElement(publicID))
					Expect(ids).NotTo(ContainElement(authenticatedID))
					Expect(ids).NotTo(ContainElement(privateID))
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

			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the public and authenticated keyboards, but not the private one", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					ids := itemIDs(resp)
					Expect(ids).To(ContainElements(publicID, authenticatedID))
					Expect(ids).NotTo(ContainElement(privateID))
				})
			})
		})
	})

	Context("given the owner has a keyboard with a purchase price", func() {
		var keyboardID string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "priced-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes the price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					Expect(purchaseOf(item)).To(HaveKeyWithValue("price", BeNumerically("==", 329.99)))
					Expect(purchaseOf(item)).To(HaveKeyWithValue("currency", "USD"))

					By("returning the keyboard in full, the same shape as a single-item GET")
					Expect(item).To(HaveKey("brand"))
					Expect(item).To(HaveKey("design"))
					Expect(purchaseOf(item)).To(HaveKey("vendor"))
				})
			})
		})

		Context("given the caller is not the owner", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits the price and currency but keeps the rest of purchase", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					purchase := purchaseOf(item)
					Expect(purchase).NotTo(BeNil(), "stripping the price must keep the rest of purchase")
					Expect(purchase).NotTo(HaveKey("price"))
					Expect(purchase).NotTo(HaveKey("currency"))
					Expect(purchase).To(HaveKey("vendor"))
				})
			})
		})
	})

	Context("given the owner has an authenticated-visibility keyboard", func() {
		var keyboardID string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "visibility-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "authenticated")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes the visibility", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					Expect(item).To(HaveKeyWithValue("visibility", "authenticated"))
				})
			})
		})

		Context("given the caller is not the owner", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits the visibility", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					Expect(item).NotTo(HaveKey("visibility"))
				})
			})
		})
	})

	Context("given the owner has a keyboard with a purchase price and show_price_to_me is false", func() {
		var keyboardID, profileUsername string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "priced-keyboard-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits the price and currency but keeps the rest of purchase", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					purchase := purchaseOf(item)
					Expect(purchase).NotTo(BeNil(), "stripping the price must keep the rest of purchase")
					Expect(purchase).NotTo(HaveKey("price"))
					Expect(purchase).NotTo(HaveKey("currency"))
					Expect(purchase).To(HaveKey("vendor"))
				})
			})
		})
	})

	Context("given the owner has a keyboard with a purchase price and show_price_to_others is true", func() {
		var keyboardID, profileUsername string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "priced-keyboard-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "EUR", "show_price_to_me": true, "show_price_to_others": true,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is not the owner", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("listing keyboards", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes the price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemByID(resp, keyboardID)
					Expect(item).NotTo(BeNil())
					Expect(purchaseOf(item)).To(HaveKeyWithValue("price", BeNumerically("==", 329.99)))
					Expect(purchaseOf(item)).To(HaveKeyWithValue("currency", "EUR"))
				})
			})
		})
	})

	DescribeTable("given an invalid limit",
		func(ctx SpecContext, limit int) {
			var err error
			resp, err = client.List(ctx, ownerID, ownerToken, limit)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
		},
		Entry("below the minimum", 0),
		Entry("above the maximum", 101),
	)

	Context("given a non-numeric limit", func() {
		When("listing keyboards", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.ListWithRawLimit(ctx, ownerID, ownerToken, "abc")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 400 with a problem+json body", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
