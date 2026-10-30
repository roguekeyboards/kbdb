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

var _ = Describe("Deleting an image from a keyboard", func() {
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

	Context("given a private keyboard owned by the caller with an image on it", func() {
		var keyboardID, imageID string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "delete-image-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())

			addResp, err := client.AddImage(ctx, ownerID, keyboardID, ownerToken, api.ImageUploadBody(approvedImageContentType))
			Expect(err).NotTo(HaveOccurred())
			Expect(addResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ImageID string `json:"image_id"`
			}
			Expect(json.NewDecoder(addResp.Body).Decode(&created)).To(Succeed())
			imageID = created.ImageID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, keyboardID, imageID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204 and the image is gone from a follow-up GetKeyboard", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer listing the image")
					getResp, err := client.Get(ctx, ownerID, keyboardID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var keyboard struct {
						Images []struct {
							ImageID string `json:"image_id"`
						} `json:"images"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&keyboard)).To(Succeed())
					Expect(keyboard.Images).To(BeEmpty())
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

			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, keyboardID, imageID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the keyboard exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, keyboardID, imageID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})

		Context("given the imageId does not exist on the keyboard", func() {
			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, keyboardID, "no-such-image-"+uuid.NewString(), ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204, idempotently", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
				})
			})
		})
	})

	Context("given the keyboard does not exist", func() {
		When("deleting an image from it", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteImage(ctx, ownerID, "no-such-keyboard-"+uuid.NewString(), "irrelevant-image", ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
