package switches_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Setting a switch's image", func() {
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

		BeforeEach(func(ctx SpecContext) {
			switchID = "set-image-switch-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given an approved content_type", func() {
				When("setting the switch's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody(approvedImageContentType))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns an upload URL that a real PUT-then-GET round-trip actually works against", func(ctx SpecContext) {
						By("returning 201 with upload_url")
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							UploadURL string `json:"upload_url"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
						Expect(created.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, err := api.DoBlob(ctx, http.MethodPut, created.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(err).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("rejecting a second PUT to the same upload URL")
						putResp, err = api.DoBlob(ctx, http.MethodPut, created.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(err).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusPreconditionFailed))

						By("showing the image, as a signed image URL, embedded in a follow-up GetSwitch")
						getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var sw struct {
							Image *struct {
								URL string `json:"url"`
							} `json:"image"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&sw)).To(Succeed())
						Expect(sw.Image).NotTo(BeNil())
						Expect(sw.Image.URL).NotTo(BeEmpty())

						By("fetching the signed image URL and getting back the exact bytes that were uploaded")
						getImageResp, err := api.DoBlob(ctx, http.MethodGet, sw.Image.URL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))

						gotBytes, err := io.ReadAll(getImageResp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(gotBytes).To(Equal(imageBytes))
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("setting the switch's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody("image/png"))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("setting the switch's image", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, switchID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the switch already has an uploaded image", func() {
				var previousImageURL string

				BeforeEach(func(ctx SpecContext) {
					firstResp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
					Expect(firstResp.StatusCode).To(Equal(http.StatusCreated))

					var first struct {
						UploadURL string `json:"upload_url"`
					}
					Expect(json.NewDecoder(firstResp.Body).Decode(&first)).To(Succeed())

					putResp, err := api.DoBlob(ctx, http.MethodPut, first.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
					Expect(err).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))

					previousImageURL = getImageURL(ctx, client, ownerID, switchID, ownerToken)
				})

				When("setting the switch's image again", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, switchID, ownerToken, api.WebPUploadBody(api.ReplacementImageBytes))
						Expect(err).NotTo(HaveOccurred())
					})

					It("replaces it with a fresh upload and removes the previous image, no need to delete first", func(ctx SpecContext) {
						By("returning 201 with a new upload_url")
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							UploadURL string `json:"upload_url"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())

						By("accepting the replacement bytes at the new upload URL")
						replacement := api.ReplacementImageBytes
						putResp, err := api.DoBlob(ctx, http.MethodPut, created.UploadURL, "image/webp", bytes.NewReader(replacement))
						Expect(err).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("serving the replacement bytes from a follow-up GetSwitch")
						getImageResp, err := api.DoBlob(ctx, http.MethodGet, getImageURL(ctx, client, ownerID, switchID, ownerToken), "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))
						gotBytes, err := io.ReadAll(getImageResp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(gotBytes).To(Equal(replacement))

						By("no longer serving the previous image's object")
						previousResp, err := api.DoBlob(ctx, http.MethodGet, previousImageURL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(previousResp.StatusCode).To(BeElementOf(http.StatusNotFound, http.StatusForbidden))
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

			When("setting the switch's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetImage(ctx, ownerID, switchID, token, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the switch exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("setting the switch's image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetImage(ctx, ownerID, switchID, "", api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the switch does not exist", func() {
		When("setting the switch's image", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.SetImage(ctx, ownerID, "no-such-switch-"+uuid.NewString(), ownerToken, api.ImageUploadBody(approvedImageContentType))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
