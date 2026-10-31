package keycapsets_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Deleting a keycap set over MCP", func() {
	var (
		client      *api.MCPClient
		result      *sdkmcp.CallToolResult
		err         error
		ownerID     string
		keycapSetID string
		deleted     bool
	)

	BeforeEach(func() {
		result = nil
		err = nil
		deleted = false
		keycapSetID = "functional-test-keycap-set-" + uuid.NewString()
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns the keycap set", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				if !deleted {
					Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
				}
			})

			When("the delete_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					if err == nil && !result.IsError {
						deleted = true
					}
				})

				It("removes the keycap set", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					By("a subsequent get_keycap_set no longer finding it")
					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeTrue())
				})
			})

			Context("given the set has a kit with an image", func() {
				BeforeEach(func(ctx SpecContext) {
					kitResult, kitErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"name":          "Base",
					})
					Expect(kitErr).NotTo(HaveOccurred())
					Expect(kitResult.IsError).To(BeFalse())
					kitID := decodeKeycapKitOutput(kitResult).KeycapKit.KitID

					setImgResult, setImgErr := client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"content_type":  approvedImageContentType,
						"size_bytes":    len(api.TestImageBytes),
					})
					Expect(setImgErr).NotTo(HaveOccurred())
					Expect(setImgResult.IsError).To(BeFalse())

					var upload struct {
						UploadURL string `json:"upload_url"`
					}
					raw, marshalErr := json.Marshal(setImgResult.StructuredContent)
					Expect(marshalErr).NotTo(HaveOccurred())
					Expect(json.Unmarshal(raw, &upload)).To(Succeed())

					putResp, putErr := api.DoBlob(ctx, http.MethodPut, upload.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
					Expect(putErr).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))
				})

				When("the delete_keycap_set tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						if err == nil && !result.IsError {
							deleted = true
						}
					})

					It("removes the set, kit and image included", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeTrue())
					})
				})
			})
		})

		Context("given the keycap set never existed", func() {
			When("the delete_keycap_set tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("succeeds, since deleting is idempotent", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
				})
			})
		})

		Context("given another user owns the keycap set", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the delete_keycap_set tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("leaves the other user's keycap set in place", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the delete_keycap_set tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
