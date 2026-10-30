package switches_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/cdn"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a switch's image", func() {
	var (
		resp       *http.Response
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
		switchID   string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewSwitchesClient()
		switchID = "delete-image-switch-" + uuid.NewString()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
	})

	Context("given a private switch owned by the caller with an image on it", func() {
		var key string

		BeforeEach(func(ctx SpecContext) {
			setResp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody(approvedImageContentType))
			Expect(err).NotTo(HaveOccurred())
			Expect(setResp.StatusCode).To(Equal(http.StatusCreated))

			img, err := db.GetSwitchImage(ctx, ownerID, switchID)
			Expect(err).NotTo(HaveOccurred())
			key = img.Path
		})

		Context("given the caller is the owner", func() {
			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, switchID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204 and the image is gone from a follow-up GetSwitch", func(ctx SpecContext) {
					By("returning 204 No Content")
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

					By("no longer showing an image")
					getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var sw struct {
						Image *struct {
							URL string `json:"url"`
						} `json:"image"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&sw)).To(Succeed())
					Expect(sw.Image).To(BeNil())
				})

				It("clears the image's cached copies", func(ctx SpecContext) {
					if !support.ImagesInvalidation() {
						Skip("this stack doesn't invalidate deleted images")
					}
					Expect(cdn.Invalidated(ctx, "/"+key)).To(BeTrue())
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
					resp, err = client.DeleteImage(ctx, ownerID, switchID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the switch exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("deleting the image", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, switchID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given a private switch owned by the caller with no image set", func() {
		When("deleting the image", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteImage(ctx, ownerID, switchID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 204, idempotently", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			})
		})
	})

	Context("given the switch does not exist", func() {
		When("deleting the image", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteImage(ctx, ownerID, "no-such-switch-"+uuid.NewString(), ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
