package keyboards_test

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

var _ = Describe("Adding an image to a keyboard over MCP", func() {
	var (
		client     *api.MCPClient
		result     *sdkmcp.CallToolResult
		err        error
		ownerID    string
		keyboardID string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		keyboardID = "functional-test-keyboard-" + uuid.NewString()
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns the keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
			})

			Context("given an approved content_type", func() {
				When("the add_keyboard_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
							"keyboard_id":  keyboardID,
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("mints an image_id and upload_url that a real PUT round-trip actually works against", func(ctx SpecContext) {
						By("returning an image_id and upload_url")
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						out := decodeAddImageOutput(result)
						Expect(out.ImageID).NotTo(BeEmpty())
						Expect(out.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(putErr).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("has_images reflecting it on a follow-up get_keyboard")
						check, checkErr := client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeGetOutput(check).Keyboard.HasImages).To(BeTrue())

						By("list_keyboard_images reporting the new image_id")
						listResult, listErr := client.CallTool(ctx, "list_keyboard_images", map[string]any{
							"keyboard_id": keyboardID,
						})
						Expect(listErr).NotTo(HaveOccurred())
						Expect(listResult.IsError).To(BeFalse())
						Expect(imageIDsOf(decodeListImagesOutput(listResult))).To(ConsistOf(out.ImageID))
					})
				})
			})

			Context("given the keyboard has no image set", func() {
				When("the list_keyboard_images tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "list_keyboard_images", map[string]any{
							"keyboard_id": keyboardID,
						})
					})

					It("succeeds with an empty images list", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						Expect(decodeListImagesOutput(result).Images).To(BeEmpty())
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("the add_keyboard_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
							"keyboard_id":  keyboardID,
							"content_type": "application/x-not-an-image",
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given size_bytes over the 1.5 MB cap", func() {
				When("the add_keyboard_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
							"keyboard_id":  keyboardID,
							"content_type": approvedImageContentType,
							"size_bytes":   api.MaxUploadBytes + 1,
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the keyboard already has 10 images", func() {
				BeforeEach(func(ctx SpecContext) {
					for range 10 {
						added, addErr := client.CallTool(ctx, "add_keyboard_image", map[string]any{
							"keyboard_id":  keyboardID,
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
						Expect(addErr).NotTo(HaveOccurred())
						Expect(added.IsError).To(BeFalse())
					}
				})

				When("the add_keyboard_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
							"keyboard_id":  keyboardID,
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})
		})

		Context("given another user owns the keyboard", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedKeyboard(ctx, otherID, keyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, otherID, keyboardID)).To(Succeed())
			})

			When("the add_keyboard_image tool is called with that keyboard_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
						"keyboard_id":  keyboardID,
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the keyboard does not exist", func() {
			When("the add_keyboard_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
						"keyboard_id":  "no-such-keyboard-" + uuid.NewString(),
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
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

		When("the add_keyboard_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "add_keyboard_image", map[string]any{
					"keyboard_id":  "irrelevant-" + uuid.NewString(),
					"content_type": approvedImageContentType,
					"size_bytes":   len(api.TestImageBytes),
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
