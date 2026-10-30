package switches_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

func getImageURL(ctx SpecContext, client *api.SwitchesClient, ownerID, switchID, token string) string {
	resp, err := client.Get(ctx, ownerID, switchID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var got struct {
		Image *struct {
			URL string `json:"url"`
		} `json:"image"`
	}
	Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
	Expect(got.Image).NotTo(BeNil())
	return got.Image.URL
}

var _ = Describe("Presigned image GET URL caching for a switch", func() {
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

		switchID = "image-cache-switch-" + uuid.NewString()
		Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())

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

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
	})

	Context("given the switch's image path has not changed", func() {
		When("getting the switch twice in a row", func() {
			It("returns the exact same presigned image URL both times", func(ctx SpecContext) {
				firstURL := getImageURL(ctx, client, ownerID, switchID, ownerToken)
				secondURL := getImageURL(ctx, client, ownerID, switchID, ownerToken)

				Expect(secondURL).To(Equal(firstURL))
			})
		})
	})

})
