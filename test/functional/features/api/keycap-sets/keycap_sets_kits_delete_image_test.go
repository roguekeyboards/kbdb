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

var _ = Describe("Deleting a keycap kit's image", func() {
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
			keycapSetID = "delete-kit-image-set-" + uuid.NewString()
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
			Context("given the kit has an image set", func() {
				var imageGetURL string

				BeforeEach(func(ctx SpecContext) {
					setResp, err := client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, api.ImageUploadBody("image/png"))
					Expect(err).NotTo(HaveOccurred())
					Expect(setResp.StatusCode).To(Equal(http.StatusCreated))

					var created struct {
						UploadURL string `json:"upload_url"`
					}
					Expect(json.NewDecoder(setResp.Body).Decode(&created)).To(Succeed())

					putResp, err := api.DoPresigned(ctx, http.MethodPut, created.UploadURL, "image/png", bytes.NewReader(api.TestImageBytes))
					Expect(err).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var set struct {
						Kits []struct {
							KitID string `json:"kit_id"`
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

				When("deleting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 204, and the object is actually gone from S3", func(ctx SpecContext) {
						By("returning 204 with no body")
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

						By("no longer showing an image on a follow-up GetKeycapSet")
						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							Kits []struct {
								KitID string `json:"kit_id"`
								Image *struct {
									URL string `json:"url"`
								} `json:"image"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(1))
						Expect(set.Kits[0].Image).To(BeNil())

						By("the previously-issued signed image URL no longer serving the object from S3")
						getImageResp, err := api.DoPresigned(ctx, http.MethodGet, imageGetURL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						// Real S3 returns 403 (not 404) for GetObject against a
						// missing key when the caller lacks s3:ListBucket, to
						// avoid leaking object existence; floci returns 404.
						Expect(getImageResp.StatusCode).To(BeElementOf(http.StatusNotFound, http.StatusForbidden))
					})
				})
			})

			Context("given the kit has no image set", func() {
				When("deleting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 204, idempotently", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
					})
				})
			})

			Context("given the kitId does not exist within the set", func() {
				When("deleting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteKitImage(ctx, ownerID, keycapSetID, "no-such-kit-"+uuid.NewString(), ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 404", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
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

			When("deleting the kit's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteKitImage(ctx, ownerID, keycapSetID, kitID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the set exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the kit's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteKitImage(ctx, ownerID, keycapSetID, kitID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the parent keycap set does not exist", func() {
		When("deleting a kit's image", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteKitImage(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), "some-kit-id", ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
