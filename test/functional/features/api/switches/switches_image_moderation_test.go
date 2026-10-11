package switches_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
	"github.com/rogueserenity/kbdb/test/functional/support/moderation"
)

var _ = Describe("Moderating an uploaded switch image", func() {
	var (
		ownerID   string
		switchID  string
		uploadURL string
		key       string
	)

	// The moderation function runs asynchronously after the upload, and its
	// first invocation includes a cold start.
	imageStatus := func(ctx SpecContext) func() (db.SwitchImage, error) {
		return func() (db.SwitchImage, error) {
			return db.GetSwitchImage(ctx, ownerID, switchID)
		}
	}

	Context("given the owner was issued an upload URL for their switch's image", func() {
		BeforeEach(func(ctx SpecContext) {
			token, id, err := api.NewAuthIdentity(ctx)
			Expect(err).NotTo(HaveOccurred())
			ownerID = id
			switchID = "moderate-image-switch-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
			DeferCleanup(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			})

			resp, err := api.NewSwitchesClient().SetImage(ctx, ownerID, switchID, token, api.JPEGUploadBody())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var created struct {
				UploadURL string `json:"upload_url"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
			uploadURL = created.UploadURL

			img, err := db.GetSwitchImage(ctx, ownerID, switchID)
			Expect(err).NotTo(HaveOccurred())
			Expect(img.Status).To(Equal("pending"))
			key = img.Path
		})

		upload := func(ctx SpecContext) {
			resp, err := api.DoPresigned(ctx, http.MethodPut, uploadURL, "image/jpeg", bytes.NewReader(api.TestJPEG))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		}

		Context("given Rekognition finds nothing in the image", func() {
			When("the image is uploaded", func() {
				BeforeEach(upload)

				It("marks the image approved", func(ctx SpecContext) {
					Eventually(imageStatus(ctx)).WithTimeout(90 * time.Second).WithPolling(time.Second).
						Should(HaveField("Status", "approved"))
				})
			})
		})

		Context("given Rekognition labels the image explicit", func() {
			BeforeEach(func() {
				if support.AIMockConfigPath() == "" {
					Skip("canned Rekognition responses need floci")
				}
				Expect(moderation.MockLabels(key,
					moderation.Label{Name: "Explicit", TaxonomyLevel: 1, Confidence: 99},
					moderation.Label{Name: "Explicit Nudity", ParentName: "Explicit", TaxonomyLevel: 2, Confidence: 97},
				)).To(Succeed())
			})

			When("the image is uploaded", func() {
				BeforeEach(upload)

				It("marks the image rejected with the label, and tags the file to expire", func(ctx SpecContext) {
					By("rejecting it with the highest-confidence label as the reason")
					Eventually(imageStatus(ctx)).WithTimeout(90 * time.Second).WithPolling(time.Second).
						Should(And(HaveField("Status", "rejected"), HaveField("RejectionReason", "Explicit")))

					By("tagging the file for the bucket's expiry rule")
					Expect(moderation.ObjectTags(ctx, key)).To(HaveKeyWithValue("moderation", "rejected"))
				})
			})
		})
	})
})
