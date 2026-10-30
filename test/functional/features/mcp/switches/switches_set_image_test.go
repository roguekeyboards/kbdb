package switches_test

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Setting a switch's image over MCP", func() {
	var (
		client   *api.MCPClient
		result   *sdkmcp.CallToolResult
		err      error
		ownerID  string
		switchID string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		switchID = "functional-test-switch-" + uuid.NewString()
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns the switch", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			})

			Context("given an approved content_type", func() {
				When("the set_switch_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_switch_image", map[string]any{
							"switch_id":    switchID,
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("mints an upload_url that a real PUT round-trip actually works against", func(ctx SpecContext) {
						By("returning an upload_url")
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						out := decodeSetImageOutput(result)
						Expect(out.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(putErr).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("has_image reflecting it on a follow-up get_switch")
						check, checkErr := client.CallTool(ctx, "get_switch", map[string]any{"switch_id": switchID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeGetOutput(check).Switch.HasImage).To(BeTrue())
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("the set_switch_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_switch_image", map[string]any{
							"switch_id":    switchID,
							"content_type": "image/png",
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

		Context("given another user owns the switch", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedSwitch(ctx, otherID, switchID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, otherID, switchID)).To(Succeed())
			})

			When("the set_switch_image tool is called with that switch_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "set_switch_image", map[string]any{
						"switch_id":    switchID,
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

		Context("given the switch does not exist", func() {
			When("the set_switch_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "set_switch_image", map[string]any{
						"switch_id":    "no-such-switch-" + uuid.NewString(),
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

		When("the set_switch_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "set_switch_image", map[string]any{
					"switch_id":    "irrelevant-" + uuid.NewString(),
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
