package keycapsets_test

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Managing a keycap kit's image over MCP", func() {
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
				keycapSetID = "kit-image-set-" + uuid.NewString()
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

			Context("given an approved content_type", func() {
				When("the set_keycap_kit_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"content_type":  approvedImageContentType,
							"size_bytes":    len(api.TestImageBytes),
						})
					})

					It("mints an upload_url that a real PUT-then-GET round-trip actually works against", func(ctx SpecContext) {
						By("returning an upload_url")
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						uploadURL := decodeUploadURL(result)
						Expect(uploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, putErr := api.DoBlob(ctx, http.MethodPut, uploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(putErr).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("has_image reflecting it on a follow-up get_keycap_set")
						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						kits := decodeKeycapSetOutput(check).KeycapSet.Kits
						Expect(kits).To(HaveLen(1))
						Expect(kits[0].HasImage).To(BeTrue())
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("the set_keycap_kit_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"content_type":  "application/x-not-an-image",
							"size_bytes":    len(api.TestImageBytes),
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the kit_id does not exist within the set", func() {
				When("the set_keycap_kit_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        "no-such-kit-" + uuid.NewString(),
							"content_type":  approvedImageContentType,
							"size_bytes":    len(api.TestImageBytes),
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the kit has no image set", func() {
				When("the delete_keycap_kit_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_kit_image", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
						})
					})

					It("succeeds, since deleting an absent image is idempotent", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
					})
				})
			})

			Context("given the kit has an image set", func() {
				BeforeEach(func(ctx SpecContext) {
					setResult, setErr := client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"content_type":  approvedImageContentType,
						"size_bytes":    len(api.TestImageBytes),
					})
					Expect(setErr).NotTo(HaveOccurred())
					Expect(setResult.IsError).To(BeFalse())
					uploadURL := decodeUploadURL(setResult)

					putResp, putErr := api.DoBlob(ctx, http.MethodPut, uploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
					Expect(putErr).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))
				})

				When("the delete_keycap_kit_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keycap_kit_image", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
						})
					})

					It("removes the image, reflected in has_image on a follow-up get_keycap_set", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						kits := decodeKeycapSetOutput(check).KeycapSet.Kits
						Expect(kits).To(HaveLen(1))
						Expect(kits[0].HasImage).To(BeFalse())
					})
				})
			})
		})

		Context("given another user owns the keycap set", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				keycapSetID = "kit-image-set-" + uuid.NewString()
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

			When("the set_keycap_kit_image tool is called with that keycap_set_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"content_type":  approvedImageContentType,
						"size_bytes":    len(api.TestImageBytes),
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the parent keycap set does not exist", func() {
			When("the set_keycap_kit_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
						"keycap_set_id": "no-such-keycap-set-" + uuid.NewString(),
						"kit_id":        "some-kit-id",
						"content_type":  approvedImageContentType,
						"size_bytes":    len(api.TestImageBytes),
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

		When("the set_keycap_kit_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "set_keycap_kit_image", map[string]any{
					"keycap_set_id": "irrelevant-" + uuid.NewString(),
					"kit_id":        "irrelevant",
					"content_type":  approvedImageContentType,
					"size_bytes":    len(api.TestImageBytes),
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})

		When("the delete_keycap_kit_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_keycap_kit_image", map[string]any{
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
