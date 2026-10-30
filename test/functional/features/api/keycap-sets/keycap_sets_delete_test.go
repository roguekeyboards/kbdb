package keycapsets_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a keycap set", func() {
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

	Context("given a private keycap set owned by the caller", func() {
		var keycapSetID string
		var deleted bool

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "delete-keycap-set-" + uuid.NewString()
			deleted = false
			Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			if !deleted {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			}
		})

		Context("given the caller is the owner", func() {
			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusNoContent {
						deleted = true
					}
				})

				It("returns 204 and the keycap set is gone", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer being gettable")
					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					Expect(getResp.StatusCode).To(Equal(http.StatusNotFound))
				})
			})
		})

		Context("given the set has a kit with an image", func() {
			var imageGetURL string

			BeforeEach(func(ctx SpecContext) {
				createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

				var created struct {
					KitID string `json:"kit_id"`
				}
				Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())

				setResp, err := client.SetKitImage(ctx, ownerID, keycapSetID, created.KitID, ownerToken, api.ImageUploadBody("image/webp"))
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

			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusNoContent {
						deleted = true
					}
				})

				It("returns 204 and the kit's image is actually gone from S3", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					getImageResp, err := api.DoBlob(ctx, http.MethodGet, imageGetURL, "", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(getImageResp.StatusCode).To(BeElementOf(http.StatusNotFound, http.StatusForbidden))
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

			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keycapSetID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, keycapSetID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the keycap set does not exist", func() {
		Context("given the caller is the owner", func() {
			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), ownerToken)
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

			When("deleting the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Delete(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), token)
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
