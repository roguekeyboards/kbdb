package switches_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Signed image URLs for a switch", func() {
	var (
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
		switchID   string
	)

	BeforeEach(func(ctx SpecContext) {
		client = api.NewSwitchesClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		switchID = "image-signing-switch-" + uuid.NewString()
		Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
	})

	Context("given the switch has an uploaded image", func() {
		BeforeEach(func(ctx SpecContext) {
			setResp, err := client.SetImage(ctx, ownerID, switchID, ownerToken, api.ImageUploadBody(approvedImageContentType))
			Expect(err).NotTo(HaveOccurred())
			Expect(setResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				UploadURL string `json:"upload_url"`
			}
			Expect(json.NewDecoder(setResp.Body).Decode(&created)).To(Succeed())

			putResp, err := api.DoBlob(ctx, http.MethodPut, created.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
			Expect(err).NotTo(HaveOccurred())
			Expect(putResp.StatusCode).To(Equal(http.StatusOK))
		})

		When("getting the switch", func() {
			var imageURL *url.URL

			BeforeEach(func(ctx SpecContext) {
				var err error
				imageURL, err = url.Parse(getImageURL(ctx, client, ownerID, switchID, ownerToken))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a signed HTTPS URL on the images domain that serves the image", func(ctx SpecContext) {
				Expect(imageURL.Scheme).To(Equal("https"))
				Expect(imageURL.Hostname()).To(Equal(support.ImagesDomain()))
				Expect(imageURL.Query().Get("Signature")).NotTo(BeEmpty())

				resp, err := api.DoBlob(ctx, http.MethodGet, imageURL.String(), "", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				got, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(api.TestImageBytes))
			})

			It("refuses the same URL without its signature", func(ctx SpecContext) {
				unsigned := *imageURL
				unsigned.RawQuery = ""

				resp, err := api.DoBlob(ctx, http.MethodGet, unsigned.String(), "", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})
		})
	})
})
