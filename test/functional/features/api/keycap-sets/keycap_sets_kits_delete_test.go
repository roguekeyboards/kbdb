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

var _ = Describe("Deleting a keycap kit", func() {
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

	Context("given a private keycap set with an existing kit", func() {
		var (
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "delete-kit-set-" + uuid.NewString()
			Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())

			createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				KitID string `json:"kit_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			kitID = created.KitID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("deleting the kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("removes the kit, embedded in the parent set", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer showing the kit in a follow-up GetKeycapSet")
					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var set struct {
						Kits []struct {
							KitID string `json:"kit_id"`
						} `json:"kits"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
					Expect(set.Kits).To(BeEmpty())
				})
			})

			Context("given the set has a second, sibling kit", func() {
				var siblingKitID string

				BeforeEach(func(ctx SpecContext) {
					createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Extension"}`)
					Expect(err).NotTo(HaveOccurred())
					Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

					var created struct {
						KitID string `json:"kit_id"`
					}
					Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
					siblingKitID = created.KitID
				})

				When("deleting the first kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("leaves the sibling kit unchanged, since the read-modify-write must not clobber other kits", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							Kits []struct {
								KitID string `json:"kit_id"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(1))
						Expect(set.Kits[0].KitID).To(Equal(siblingKitID))
					})
				})
			})

			Context("given the kit has an image set", func() {
				var imageGetURL string

				BeforeEach(func(ctx SpecContext) {
					setResp, err := client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, api.ImageUploadBody("image/webp"))
					Expect(err).NotTo(HaveOccurred())
					Expect(setResp.StatusCode).To(Equal(http.StatusCreated))

					var upload struct {
						UploadURL string `json:"upload_url"`
					}
					Expect(json.NewDecoder(setResp.Body).Decode(&upload)).To(Succeed())

					putResp, err := api.DoBlob(ctx, http.MethodPut, upload.UploadURL, "image/webp", bytes.NewReader(api.TestImageBytes))
					Expect(err).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var set struct {
						Kits []struct {
							Image *struct {
								URL string `json:"url"`
							} `json:"image"`
						} `json:"kits"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
					Expect(set.Kits).To(HaveLen(1))
					Expect(set.Kits[0].Image).NotTo(BeNil())
					imageGetURL = set.Kits[0].Image.URL
				})

				When("deleting the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 204 and the kit's image is actually gone from S3", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

						getImageResp, err := api.DoBlob(ctx, http.MethodGet, imageGetURL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(BeElementOf(http.StatusNotFound, http.StatusForbidden))
					})
				})
			})

			Context("given the kitId does not exist within the set", func() {
				When("deleting the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, "no-such-kit-"+uuid.NewString(), ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 204, since deleting a kit that's already absent is a no-op", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
					})
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

			When("deleting the kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the set exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given a private keycap set whose primary kit is the one being deleted", func() {
		var (
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "delete-primary-kit-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		When("deleting the primary kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteKit(ctx, ownerID, keycapSetID, kitID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("clears primary_kit_id on a follow-up GetKeycapSet", func(ctx SpecContext) {
				Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

				getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
				Expect(err).NotTo(HaveOccurred())

				var set struct {
					PrimaryKitID *string `json:"primary_kit_id"`
				}
				Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
				Expect(set.PrimaryKitID).To(BeNil())
			})
		})
	})

	Context("given the parent keycap set does not exist", func() {
		When("deleting a kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteKit(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), "some-kit-id", ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
