package switches_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/cdn"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
	"github.com/roguekeyboards/kbdb/test/functional/support/moderation"
)

var _ = Describe("Taking down a switch image", func() {
	var (
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
		switchID   string
		key        string
		imageURL   string
	)

	BeforeEach(func(ctx SpecContext) {
		client = api.NewSwitchesClient()
		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		switchID = "takedown-switch-" + uuid.NewString()
		Expect(db.SeedSwitch(ctx, ownerID, switchID, "public")).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})
	})

	Context("given an approved image whose URL has been fetched", func() {
		BeforeEach(func(ctx SpecContext) {
			resp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody("image/webp"))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var created struct {
				UploadURL string `json:"upload_url"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())

			putResp, err := api.DoBlob(ctx, http.MethodPut, created.UploadURL, "image/webp", bytes.NewReader(api.TestImageBytes))
			Expect(err).NotTo(HaveOccurred())
			Expect(putResp.StatusCode).To(Equal(http.StatusOK))

			Eventually(func() (db.SwitchImage, error) {
				return db.GetSwitchImage(ctx, ownerID, switchID)
			}).WithTimeout(90 * time.Second).WithPolling(time.Second).Should(HaveField("Status", "approved"))

			img, err := db.GetSwitchImage(ctx, ownerID, switchID)
			Expect(err).NotTo(HaveOccurred())
			key = img.Path

			imageURL = getImageURL(ctx, client, ownerID, switchID, ownerToken)
			getResp, err := api.DoBlob(ctx, http.MethodGet, imageURL, "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))
		})

		When("an operator takes it down", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(moderation.Reject(ctx, key, "takedown")).To(Succeed())
			})

			It("marks the image rejected", func(ctx SpecContext) {
				Expect(db.GetSwitchImage(ctx, ownerID, switchID)).To(HaveField("Status", "rejected"))
			})

			It("clears the image's cached copies", func(ctx SpecContext) {
				Expect(cdn.Invalidated(ctx, "/"+key)).To(BeTrue())
			})

			It("refuses the image's old URL", func(ctx SpecContext) {
				if !support.IsCI() {
					Skip("floci's bucket policies aren't offered s3:ExistingObjectTag, so the rejected-image deny never applies")
				}
				Eventually(func() (int, error) {
					resp, err := api.DoBlob(ctx, http.MethodGet, imageURL, "", nil)
					if err != nil {
						return 0, err
					}
					return resp.StatusCode, nil
				}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusForbidden))
			})
		})
	})
})
