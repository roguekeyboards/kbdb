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

var _ = Describe("Deleting a keycap kit over MCP", func() {
	var (
		client      *api.MCPClient
		result      *sdkmcp.CallToolResult
		err         error
		ownerID     string
		keycapSetID string
		kitID       string
	)

	BeforeEach(func() {
		result = nil
		err = nil
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns a keycap set with an existing kit", func() {
			BeforeEach(func(ctx SpecContext) {
				keycapSetID = "delete-kit-set-" + uuid.NewString()
				Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())

				createResult, createErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
					"keycap_set_id": keycapSetID,
					"name":          "Base",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				kitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the delete_keycap_kit tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
					})
				})

				It("removes the kit, embedded in the parent set", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(decodeKeycapSetOutput(check).KeycapSet.Kits).To(BeEmpty())
				})
			})

			Context("given the set has a second, sibling kit", func() {
				var siblingKitID string

				BeforeEach(func(ctx SpecContext) {
					createResult, createErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"name":          "Extension",
					})
					Expect(createErr).NotTo(HaveOccurred())
					Expect(createResult.IsError).To(BeFalse())
					siblingKitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
				})

				When("the delete_keycap_kit tool is called for the first kit", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
						})
					})

					It("leaves the sibling kit unchanged", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())

						kits := decodeKeycapSetOutput(check).KeycapSet.Kits
						Expect(kits).To(HaveLen(1))
						Expect(kits[0].KitID).To(Equal(siblingKitID))
					})
				})
			})

			Context("given the kit has an image set", func() {
				BeforeEach(func(ctx SpecContext) {
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

				When("the delete_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
						})
					})

					It("removes the kit, image included", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())

						kits := decodeKeycapSetOutput(check).KeycapSet.Kits
						for _, kit := range kits {
							Expect(kit.KitID).NotTo(Equal(kitID))
						}
					})
				})
			})

			Context("given the kit_id does not exist within the set", func() {
				When("the delete_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        "no-such-kit-" + uuid.NewString(),
						})
					})

					It("succeeds, since deleting an absent kit is idempotent", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
					})
				})
			})
		})

		Context("given the caller owns a keycap set whose primary kit is the one being deleted", func() {
			BeforeEach(func(ctx SpecContext) {
				keycapSetID = "delete-primary-kit-set-" + uuid.NewString()
				kitID = "kit-" + uuid.NewString()
				Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the delete_keycap_kit tool is called for the primary kit", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
					})
				})

				It("clears primary_kit_id on a follow-up get_keycap_set", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(decodeKeycapSetOutput(check).KeycapSet.PrimaryKitID).To(BeNil())
				})
			})
		})

		Context("given another user owns the keycap set", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				keycapSetID = "delete-kit-set-" + uuid.NewString()
				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "public")).To(Succeed())

				otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)
				createResult, createErr := otherClient.CallTool(ctx, "create_keycap_kit", map[string]any{
					"keycap_set_id": keycapSetID,
					"name":          "Base",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				kitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the delete_keycap_kit tool is called with that keycap_set_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
					})
				})

				It("returns an MCP tool error result and leaves the kit in place", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(decodeKeycapSetOutput(check).KeycapSet.Kits).To(HaveLen(1))
				})
			})
		})

		Context("given the parent keycap set does not exist", func() {
			When("the delete_keycap_kit tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
						"keycap_set_id": "no-such-keycap-set-" + uuid.NewString(),
						"kit_id":        "some-kit-id",
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the delete_keycap_kit tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_keycap_kit", map[string]any{
					"keycap_set_id": "irrelevant-" + uuid.NewString(),
					"kit_id":        "irrelevant",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
