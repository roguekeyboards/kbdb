package keycapsets_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

func kitsOf(item map[string]any) []map[string]any {
	raw, _ := item["kits"].([]any)
	kits := make([]map[string]any, 0, len(raw))
	for _, k := range raw {
		if kit, ok := k.(map[string]any); ok {
			kits = append(kits, kit)
		}
	}

	return kits
}

type listedKit struct {
	KitID string `json:"kit_id"`
	Image *struct {
		URL string `json:"url"`
	} `json:"image"`
}

type listedKeycapSet struct {
	ID           string      `json:"id"`
	PrimaryKitID *string     `json:"primary_kit_id"`
	Kits         []listedKit `json:"kits"`
}

func (s listedKeycapSet) kit(id string) *listedKit {
	for i := range s.Kits {
		if s.Kits[i].KitID == id {
			return &s.Kits[i]
		}
	}

	return nil
}

var _ = Describe("Listing keycap sets", func() {
	var (
		resp       *http.Response
		client     *api.KeycapSetsClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeycapSetsClient()

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

	Context("given the owner has keycap sets at every visibility tier", func() {
		var publicID, authenticatedID, privateID string

		BeforeEach(func(ctx SpecContext) {
			publicID = "public-keycap-set-" + uuid.NewString()
			authenticatedID = "authenticated-keycap-set-" + uuid.NewString()
			privateID = "private-keycap-set-" + uuid.NewString()

			Expect(db.SeedKeycapSet(ctx, ownerID, publicID, "public")).To(Succeed())
			Expect(db.SeedKeycapSet(ctx, ownerID, authenticatedID, "authenticated")).To(Succeed())
			Expect(db.SeedKeycapSet(ctx, ownerID, privateID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, publicID)).To(Succeed())
			Expect(db.DeleteKeycapSet(ctx, ownerID, authenticatedID)).To(Succeed())
			Expect(db.DeleteKeycapSet(ctx, ownerID, privateID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns keycap sets at every visibility tier", func() {
					By("returning 200 OK")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					By("including all three seeded keycap sets")
					Expect(itemIDs(resp)).To(ContainElements(publicID, authenticatedID, privateID))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, "", -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns only the public keycap set", func() {
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

			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the public and authenticated keycap sets, but not the private one", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					ids := itemIDs(resp)
					Expect(ids).To(ContainElements(publicID, authenticatedID))
					Expect(ids).NotTo(ContainElement(privateID))
				})
			})
		})
	})

	Context("given the owner has a keycap set with a primary kit that has an image", func() {
		var (
			keycapSetID string
			kitID       string
			imageBytes  []byte
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "primary-kit-image-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "public")).To(Succeed())

			setImageResp, err := client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, api.ImageUploadBody("image/png"))
			Expect(err).NotTo(HaveOccurred())
			Expect(setImageResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				UploadURL string `json:"upload_url"`
			}
			Expect(json.NewDecoder(setImageResp.Body).Decode(&created)).To(Succeed())

			imageBytes = api.TestImageBytes
			putResp, err := api.DoBlob(ctx, http.MethodPut, created.UploadURL, "image/png", bytes.NewReader(imageBytes))
			Expect(err).NotTo(HaveOccurred())
			Expect(putResp.StatusCode).To(Equal(http.StatusOK))
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		When("listing keycap sets", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.List(ctx, ownerID, ownerToken, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the primary kit's id and a working presigned image URL on that kit", func(ctx SpecContext) {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var page struct {
					Items []listedKeycapSet `json:"items"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&page)).To(Succeed())

				var found bool
				for _, item := range page.Items {
					if item.ID != keycapSetID {
						continue
					}
					found = true
					Expect(item.PrimaryKitID).To(HaveValue(Equal(kitID)))
					kit := item.kit(kitID)
					Expect(kit).NotTo(BeNil())
					Expect(kit.Image).NotTo(BeNil())
					Expect(kit.Image.URL).NotTo(BeEmpty())

					getImageResp, err := api.DoBlob(ctx, http.MethodGet, kit.Image.URL, "", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))
				}
				Expect(found).To(BeTrue(), "expected to find seeded keycap set %q in the list", keycapSetID)
			})
		})
	})

	Context("given the owner has a keycap set with a primary kit that has no image", func() {
		var (
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "primary-kit-no-image-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		When("listing keycap sets", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.List(ctx, ownerID, ownerToken, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the primary kit's id, with no image on that kit", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var page struct {
					Items []listedKeycapSet `json:"items"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&page)).To(Succeed())

				var found bool
				for _, item := range page.Items {
					if item.ID != keycapSetID {
						continue
					}
					found = true
					Expect(item.PrimaryKitID).To(HaveValue(Equal(kitID)))
					kit := item.kit(kitID)
					Expect(kit).NotTo(BeNil())
					Expect(kit.Image).To(BeNil())
				}
				Expect(found).To(BeTrue(), "expected to find seeded keycap set %q in the list", keycapSetID)
			})
		})
	})

	Context("given the owner has a keycap set whose primary kit no longer exists", func() {
		var keycapSetID string

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "dangling-primary-kit-keycap-set-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithDanglingPrimaryKit(ctx, ownerID, keycapSetID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		When("listing keycap sets", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.List(ctx, ownerID, ownerToken, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("reports a null primary_kit_id for that set", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var page struct {
					Items []listedKeycapSet `json:"items"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&page)).To(Succeed())

				var found bool
				for _, item := range page.Items {
					if item.ID != keycapSetID {
						continue
					}
					found = true
					Expect(item.PrimaryKitID).To(BeNil())
				}
				Expect(found).To(BeTrue(), "expected to find seeded keycap set %q in the list", keycapSetID)
			})
		})
	})

	Context("given the owner has a keycap set with a priced kit", func() {
		var (
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "priced-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		itemTotalCost := func(r *http.Response, id string) map[string]any {
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

		Context("given the caller is the owner", func() {
			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes the total_cost", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemTotalCost(resp, keycapSetID)
					Expect(item).NotTo(BeNil())
					Expect(item).To(HaveKeyWithValue("total_cost", BeNumerically("==", 85.0)))
					Expect(item).To(HaveKeyWithValue("currency", "USD"))
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

			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits the total_cost, its currency and every kit price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					item := itemTotalCost(resp, keycapSetID)
					Expect(item).NotTo(BeNil())
					Expect(item).NotTo(HaveKey("total_cost"))
					Expect(item).NotTo(HaveKey("currency"))
					Expect(kitsOf(item)).NotTo(BeEmpty())
					for _, kit := range kitsOf(item) {
						Expect(kit["purchase"]).To(HaveKey("vendor"), "stripping the price must keep the rest of the kit's purchase")
						Expect(kit["purchase"]).NotTo(HaveKey("price"))
						Expect(kit["purchase"]).NotTo(HaveKey("currency"))
					}
				})
			})
		})
	})

	Context("given the owner has a priced kit and show_price_to_me is false", func() {
		var (
			keycapSetID, kitID, profileUsername string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "priced-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits the total_cost, its currency and every kit price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var page struct {
						Items []map[string]any `json:"items"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&page)).To(Succeed())
					var item map[string]any
					for _, i := range page.Items {
						if i["id"] == keycapSetID {
							item = i
						}
					}
					Expect(item).NotTo(BeNil())
					Expect(item).NotTo(HaveKey("total_cost"))
					Expect(item).NotTo(HaveKey("currency"))
					Expect(kitsOf(item)).NotTo(BeEmpty())
					for _, kit := range kitsOf(item) {
						Expect(kit["purchase"]).To(HaveKey("vendor"), "stripping the price must keep the rest of the kit's purchase")
						Expect(kit["purchase"]).NotTo(HaveKey("price"))
						Expect(kit["purchase"]).NotTo(HaveKey("currency"))
					}
				})
			})
		})
	})

	Context("given the owner has a priced kit and show_price_to_others is true", func() {
		var (
			keycapSetID, kitID, profileUsername string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "priced-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "EUR", "show_price_to_me": true, "show_price_to_others": true,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is not the owner", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("listing keycap sets", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes the total_cost", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var page struct {
						Items []map[string]any `json:"items"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&page)).To(Succeed())
					var item map[string]any
					for _, i := range page.Items {
						if i["id"] == keycapSetID {
							item = i
						}
					}
					Expect(item).NotTo(BeNil())
					Expect(item).To(HaveKeyWithValue("total_cost", BeNumerically("==", 85.0)))
					Expect(item).To(HaveKeyWithValue("currency", "EUR"))
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
		When("listing keycap sets", func() {
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
