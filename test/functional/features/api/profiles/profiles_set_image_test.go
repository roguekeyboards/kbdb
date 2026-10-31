package profiles_test

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

const approvedImageContentType = "image/webp"

var _ = Describe("Setting a profile's avatar", func() {
	var (
		resp       *http.Response
		client     *api.ProfilesClient
		ownerID    string
		ownerToken string
		username   string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewProfilesClient()
		username = "u" + uuid.NewString()[:8]

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given the caller has a profile", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:     username,
				Discoverable: true,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given an approved content_type", func() {
				When("setting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody(approvedImageContentType))
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

						By("showing the avatar, as a signed image URL, embedded in a follow-up GetProfile")
						getResp, err := client.Get(ctx, ownerID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var p struct {
							Avatar *struct {
								URL string `json:"url"`
							} `json:"avatar"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&p)).To(Succeed())
						Expect(p.Avatar).NotTo(BeNil())
						Expect(p.Avatar.URL).NotTo(BeEmpty())

						By("fetching the signed image URL and getting back the exact bytes that were uploaded")
						getImageResp, err := api.DoBlob(ctx, http.MethodGet, p.Avatar.URL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))

						gotBytes, err := io.ReadAll(getImageResp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(gotBytes).To(Equal(imageBytes))
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("setting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody("application/x-not-an-image"))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("setting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the avatar is already set", func() {
				BeforeEach(func(ctx SpecContext) {
					firstResp, err := client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
					Expect(firstResp.StatusCode).To(Equal(http.StatusCreated))
				})

				When("setting the avatar again", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody(approvedImageContentType))
						Expect(err).NotTo(HaveOccurred())
					})

					It("replaces it, no need to delete first", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))
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

			When("setting the avatar", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetImage(ctx, ownerID, token, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing whose profile it is", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("setting the avatar", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetImage(ctx, ownerID, "", api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})

		Context("given the path identifier is a username, not the caller's subject", func() {
			When("setting the avatar", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.SetImage(ctx, username, ownerToken, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404 - writes address the profile by subject only", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})

	Context("given the caller has no profile", func() {
		When("setting the avatar", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody(approvedImageContentType))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
