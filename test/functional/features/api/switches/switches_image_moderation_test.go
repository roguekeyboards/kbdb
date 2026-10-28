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

	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
	"github.com/rogueserenity/kbdb/test/functional/support/moderation"
)

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

	issueUpload := func(ctx SpecContext, body string) {
		GinkgoHelper()
		resp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, body)
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
	}

	put := func(ctx SpecContext, contentType string, data []byte) {
		GinkgoHelper()
		resp, err := api.DoBlob(ctx, http.MethodPut, uploadURL, contentType, bytes.NewReader(data))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	}

	upload := func(ctx SpecContext) {
		put(ctx, "image/webp", api.TestImageBytes)
	}

	awaitStatus := func(ctx SpecContext, want ...types.GomegaMatcher) {
		GinkgoHelper()
		Eventually(func() (db.SwitchImage, error) {
			return db.GetSwitchImage(ctx, ownerID, switchID)
		}).WithTimeout(90 * time.Second).WithPolling(time.Second).Should(And(want...))
	}

	BeforeEach(func(ctx SpecContext) {
		client = api.NewSwitchesClient()
		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
		otherToken, _, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		switchID = "moderate-image-switch-" + uuid.NewString()
		DeferCleanup(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})
	})

	Context("given a public switch whose owner was issued an image upload URL", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "public")).To(Succeed())
			issueUpload(ctx, api.ImageUploadBody("image/webp"))
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

				It("marks the image approved", func(ctx SpecContext) {
					awaitStatus(ctx, HaveField("Status", "approved"))
				})
			})
		})
	})

	Context("given a public switch", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "public")).To(Succeed())
		})

		Context("given its owner was issued an upload URL for a busy 2560px WebP", func() {
			var data []byte

			BeforeEach(func(ctx SpecContext) {
				data = api.BusyWebP()
				Expect(len(data)).To(BeNumerically("<=", api.MaxUploadBytes), "the WebP must fit the upload cap")
				Expect(api.FullSizeJPEGLen(data)).To(BeNumerically(">", api.RekognitionBytesLimit), "only a shrunk JPEG may fit Rekognition's Bytes limit")
				issueUpload(ctx, api.WebPUploadBody(data))
			})

			When("the image is uploaded", func() {
				BeforeEach(func(ctx SpecContext) {
					put(ctx, "image/webp", data)
				})

				It("shrinks it enough for Rekognition and marks it approved", func(ctx SpecContext) {
					awaitStatus(ctx, HaveField("Status", "approved"))
				})
			})
		})

		Context("given its owner was issued an upload URL for a WebP over 2560px", func() {
			var data []byte

			BeforeEach(func(ctx SpecContext) {
				data = api.OversizedWebP()
				issueUpload(ctx, api.WebPUploadBody(data))
			})

			When("the image is uploaded", func() {
				BeforeEach(func(ctx SpecContext) {
					put(ctx, "image/webp", data)
				})

				It("rejects it as too_large, and tags the file to expire", func(ctx SpecContext) {
					By("rejecting it as too_large")
					awaitStatus(ctx, HaveField("Status", "rejected"), HaveField("RejectionReason", "too_large"))

					By("tagging the file for the bucket's expiry rule")
					Expect(moderation.ObjectTags(ctx, key)).To(HaveKeyWithValue("moderation", "rejected"))
				})
			})
		})

		Context("given its owner was issued a WebP upload URL for a JPEG", func() {
			BeforeEach(func(ctx SpecContext) {
				issueUpload(ctx, api.WebPUploadBody(api.TestJPEG))
			})

			When("the JPEG is uploaded as a WebP", func() {
				BeforeEach(func(ctx SpecContext) {
					put(ctx, "image/webp", api.TestJPEG)
				})

				It("rejects it as not_webp, and tags the file to expire", func(ctx SpecContext) {
					By("rejecting it as not_webp")
					awaitStatus(ctx, HaveField("Status", "rejected"), HaveField("RejectionReason", "not_webp"))

					By("tagging the file for the bucket's expiry rule")
					Expect(moderation.ObjectTags(ctx, key)).To(HaveKeyWithValue("moderation", "rejected"))
				})
			})
		})
	})

	Context("given a public switch whose image was approved", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedSwitchWithModeratedImage(ctx, ownerID, switchID, "public", "approved", "")).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the switch", func() {
				It("shows the image as approved, with a URL", func(ctx SpecContext) {
					img := getImage(ctx, ownerToken)
					Expect(img).NotTo(BeNil())
					Expect(img.Status).To(HaveValue(Equal("approved")))
					Expect(img.URL).NotTo(BeNil())
					Expect(img.RejectionReason).To(BeNil())
				})
			})
		})

		Context("given the caller is another user", func() {
			When("getting the switch", func() {
				It("shows the image's URL, without a status", func(ctx SpecContext) {
					img := getImage(ctx, otherToken)
					Expect(img).NotTo(BeNil())
					Expect(img.URL).NotTo(BeNil())
					Expect(img.Status).To(BeNil())
				})
			})
		})
	})

	Context("given a public switch whose image was rejected", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedSwitchWithModeratedImage(ctx, ownerID, switchID, "public", "rejected", "Explicit")).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the switch", func() {
				It("shows the rejection reason, with no URL", func(ctx SpecContext) {
					img := getImage(ctx, ownerToken)
					Expect(img).NotTo(BeNil())
					Expect(img.Status).To(HaveValue(Equal("rejected")))
					Expect(img.RejectionReason).To(HaveValue(Equal("Explicit")))
					Expect(img.URL).To(BeNil())
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
})
