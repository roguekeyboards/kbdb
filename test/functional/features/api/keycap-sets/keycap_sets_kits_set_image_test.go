package keycapsets_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

var _ = Describe("Setting a keycap kit's image", func() {
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
			keycapSetID = "set-kit-image-set-" + uuid.NewString()
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
			Context("given an approved content_type", func() {
				When("setting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, api.ImageUploadBody("image/png"))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns a presigned upload URL that a real PUT-then-GET round-trip actually works against", func(ctx SpecContext) {
						By("returning 201 with an upload_url")
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							UploadURL string `json:"upload_url"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
						Expect(created.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, err := api.DoPresigned(ctx, http.MethodPut, created.UploadURL, "image/png", bytes.NewReader(imageBytes))
						Expect(err).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("showing the kit's image, as a signed image URL, embedded in a follow-up GetKeycapSet")
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
						Expect(set.Kits[0].KitID).To(Equal(kitID))
						Expect(set.Kits[0].Image).NotTo(BeNil())
						Expect(set.Kits[0].Image.URL).NotTo(BeEmpty())

						By("fetching the signed image URL and getting back the exact bytes that were uploaded")
						getImageResp, err := api.DoPresigned(ctx, http.MethodGet, set.Kits[0].Image.URL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))

						gotBytes, err := io.ReadAll(getImageResp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(gotBytes).To(Equal(imageBytes))
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("setting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, api.ImageUploadBody("application/x-not-an-image"))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("setting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, kitID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the kitId does not exist within the set", func() {
				When("setting the kit's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, "no-such-kit-"+uuid.NewString(), ownerToken, api.ImageUploadBody("image/png"))
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

			When("setting the kit's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, kitID, token, api.ImageUploadBody("image/png"))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the set exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("setting the kit's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetKitImage(ctx, ownerID, keycapSetID, kitID, "", api.ImageUploadBody("image/png"))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the parent keycap set does not exist", func() {
		When("setting a kit's image", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.SetKitImage(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), "some-kit-id", ownerToken, api.ImageUploadBody("image/png"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
