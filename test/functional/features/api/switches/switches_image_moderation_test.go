package switches_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
	"github.com/rogueserenity/kbdb/test/functional/support/moderation"
)

// switchImage is GET .../switches/{switchId}'s image field.
type switchImage struct {
	URL             *string `json:"url"`
	Status          *string `json:"status"`
	RejectionReason *string `json:"rejection_reason"`
}

var _ = Describe("Moderating an uploaded switch image", func() {
	var (
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
		otherToken string
		switchID   string
		uploadURL  string
		key        string
	)

	getImage := func(ctx context.Context, token string) *switchImage {
		resp, err := client.Get(ctx, ownerID, switchID, token)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var sw struct {
			Image *switchImage `json:"image"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&sw)).To(Succeed())
		return sw.Image
	}

	upload := func(ctx SpecContext) {
		resp, err := api.DoPresigned(ctx, http.MethodPut, uploadURL, "image/jpeg", bytes.NewReader(api.TestJPEG))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	}

	// The moderation function runs asynchronously after the upload, and its
	// first invocation includes a cold start.
	awaitStatus := func(ctx SpecContext, want ...types.GomegaMatcher) {
		GinkgoHelper()
		Eventually(func() (db.SwitchImage, error) {
			return db.GetSwitchImage(ctx, ownerID, switchID)
		}).WithTimeout(90 * time.Second).WithPolling(time.Second).Should(And(want...))
	}

	Context("given a public switch whose owner was issued an image upload URL", func() {
		BeforeEach(func(ctx SpecContext) {
			client = api.NewSwitchesClient()
			var err error
			ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
			Expect(err).NotTo(HaveOccurred())
			otherToken, _, err = api.NewAuthIdentity(ctx)
			Expect(err).NotTo(HaveOccurred())

			switchID = "moderate-image-switch-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "public")).To(Succeed())
			DeferCleanup(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			})

			resp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, api.JPEGUploadBody())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var created struct {
				UploadURL string `json:"upload_url"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
			uploadURL = created.UploadURL

			img, err := db.GetSwitchImage(ctx, ownerID, switchID)
			Expect(err).NotTo(HaveOccurred())
			key = img.Path
		})

		Context("given nothing has been uploaded to it yet", func() {
			Context("given the caller is the owner", func() {
				When("getting the switch", func() {
					It("shows the image as pending, with a URL", func(ctx SpecContext) {
						img := getImage(ctx, ownerToken)
						Expect(img).NotTo(BeNil())
						Expect(img.Status).To(HaveValue(Equal("pending")))
						Expect(img.URL).NotTo(BeNil())
						Expect(img.RejectionReason).To(BeNil())
					})
				})
			})

			Context("given the caller is another user", func() {
				When("getting the switch", func() {
					It("shows no image", func(ctx SpecContext) {
						Expect(getImage(ctx, otherToken)).To(BeNil())
					})
				})
			})
		})

		Context("given Rekognition finds nothing in the image", func() {
			When("the image is uploaded", func() {
				BeforeEach(upload)

				It("approves it, and then shows it to everyone", func(ctx SpecContext) {
					By("marking the image approved")
					awaitStatus(ctx, HaveField("Status", "approved"))

					By("showing another user its URL, without a status")
					img := getImage(ctx, otherToken)
					Expect(img).NotTo(BeNil())
					Expect(img.URL).NotTo(BeNil())
					Expect(img.Status).To(BeNil())
				})
			})
		})

		Context("given Rekognition labels the image explicit", func() {
			BeforeEach(func(ctx SpecContext) {
				if support.AIMockConfigPath() == "" {
					Skip("canned Rekognition responses need floci")
				}
				Expect(moderation.MockLabels(ctx, key,
					moderation.Label{Name: "Explicit", TaxonomyLevel: 1, Confidence: 99},
					moderation.Label{Name: "Explicit Nudity", ParentName: "Explicit", TaxonomyLevel: 2, Confidence: 97},
				)).To(Succeed())
			})

			When("the image is uploaded", func() {
				BeforeEach(upload)

				It("rejects it, tags the file to expire, and shows only the owner why", func(ctx SpecContext) {
					By("rejecting it with the highest-confidence label as the reason")
					awaitStatus(ctx, HaveField("Status", "rejected"), HaveField("RejectionReason", "Explicit"))

					By("tagging the file for the bucket's expiry rule")
					Expect(moderation.ObjectTags(ctx, key)).To(HaveKeyWithValue("moderation", "rejected"))

					By("showing the owner the reason and no URL")
					img := getImage(ctx, ownerToken)
					Expect(img).NotTo(BeNil())
					Expect(img.Status).To(HaveValue(Equal("rejected")))
					Expect(img.RejectionReason).To(HaveValue(Equal("Explicit")))
					Expect(img.URL).To(BeNil())

					By("showing another user no image")
					Expect(getImage(ctx, otherToken)).To(BeNil())
				})
			})
		})
	})
})
