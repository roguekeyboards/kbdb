package builds_test

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

// buildImageIDs GETs the build and returns its image ids in the order the
// API presents them.
func buildImageIDs(ctx SpecContext, client *api.BuildsClient, ownerID, buildID, token string) []string {
	GinkgoHelper()
	getResp, err := client.Get(ctx, ownerID, buildID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = getResp.Body.Close() }()
	Expect(getResp.StatusCode).To(Equal(http.StatusOK))

	var b struct {
		Images []struct {
			ImageID string `json:"image_id"`
		} `json:"images"`
	}
	Expect(json.NewDecoder(getResp.Body).Decode(&b)).To(Succeed())

	ids := make([]string, len(b.Images))
	for i, img := range b.Images {
		ids[i] = img.ImageID
	}
	return ids
}

var _ = Describe("Adding an image to a build", func() {
	var (
		resp       *http.Response
		client     *api.BuildsClient
		ownerID    string
		ownerToken string
		keyboardID string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewBuildsClient()
		keyboardID = "build-fixture-keyboard-" + uuid.NewString()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given a private build owned by the caller", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = "add-image-build-" + uuid.NewString()
			Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given an approved content_type", func() {
				When("adding an image to the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.AddImage(ctx, ownerID, buildID, ownerToken, api.ImageUploadBody(approvedImageContentType))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns an image_id and an upload URL that a real PUT-then-GET round-trip actually works against", func(ctx SpecContext) {
						By("returning 201 with image_id and upload_url")
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							ImageID   string `json:"image_id"`
							UploadURL string `json:"upload_url"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
						Expect(created.ImageID).NotTo(BeEmpty())
						Expect(created.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, err := api.DoPresigned(ctx, http.MethodPut, created.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(err).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("showing the image, as a signed image URL, embedded in a follow-up GetBuild")
						getResp, err := client.Get(ctx, ownerID, buildID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var build struct {
							Images []struct {
								ImageID string `json:"image_id"`
								URL     string `json:"url"`
							} `json:"images"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&build)).To(Succeed())
						Expect(build.Images).To(HaveLen(1))
						Expect(build.Images[0].ImageID).To(Equal(created.ImageID))
						Expect(build.Images[0].URL).NotTo(BeEmpty())

						By("fetching the signed image URL and getting back the exact bytes that were uploaded")
						getImageResp, err := api.DoPresigned(ctx, http.MethodGet, build.Images[0].URL, "", nil)
						Expect(err).NotTo(HaveOccurred())
						Expect(getImageResp.StatusCode).To(Equal(http.StatusOK))

						gotBytes, err := io.ReadAll(getImageResp.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(gotBytes).To(Equal(imageBytes))
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("adding an image to the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.AddImage(ctx, ownerID, buildID, ownerToken, api.ImageUploadBody("application/x-not-an-image"))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("adding an image to the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.AddImage(ctx, ownerID, buildID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the build already has 10 images", func() {
				BeforeEach(func(ctx SpecContext) {
					for range 10 {
						addResp, err := client.AddImage(ctx, ownerID, buildID, ownerToken, api.ImageUploadBody(approvedImageContentType))
						Expect(err).NotTo(HaveOccurred())
						_ = addResp.Body.Close()
						Expect(addResp.StatusCode).To(Equal(http.StatusCreated))
					}
				})

				When("adding an image to the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.AddImage(ctx, ownerID, buildID, ownerToken, api.ImageUploadBody(approvedImageContentType))
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 409 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusConflict))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given several images added in sequence", func() {
				When("listing them, then deleting a middle one", func() {
					It("returns them in add order, and the order survives the delete", func(ctx SpecContext) {
						By("adding three images in order")
						ids := make([]string, 3)
						for i := range ids {
							addResp, err := client.AddImage(ctx, ownerID, buildID, ownerToken,
								api.ImageUploadBody(approvedImageContentType))
							Expect(err).NotTo(HaveOccurred())
							Expect(addResp.StatusCode).To(Equal(http.StatusCreated))
							var created struct {
								ImageID string `json:"image_id"`
							}
							Expect(json.NewDecoder(addResp.Body).Decode(&created)).To(Succeed())
							ids[i] = created.ImageID
						}

						By("GET returning them in add order")
						Expect(buildImageIDs(ctx, client, ownerID, buildID, ownerToken)).To(Equal(ids))

						By("deleting the middle image")
						delResp, err := client.DeleteImage(ctx, ownerID, buildID, ids[1], ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(delResp.StatusCode).To(Equal(http.StatusNoContent))

						By("GET returning the remaining two in their original relative order")
						Expect(buildImageIDs(ctx, client, ownerID, buildID, ownerToken)).To(Equal([]string{ids[0], ids[2]}))
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

			When("adding an image to the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.AddImage(ctx, ownerID, buildID, token, api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the build exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("adding an image to the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.AddImage(ctx, ownerID, buildID, "", api.ImageUploadBody(approvedImageContentType))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the build does not exist", func() {
		When("adding an image to the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.AddImage(ctx, ownerID, "no-such-build-"+uuid.NewString(), ownerToken, api.ImageUploadBody(approvedImageContentType))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
