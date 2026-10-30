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

var _ = Describe("Deleting a switch's image over MCP", func() {
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

		Context("given the caller owns the switch with an image on it", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())

				setResult, setErr := client.CallTool(ctx, "set_switch_image", map[string]any{
					"switch_id":    switchID,
					"content_type": approvedImageContentType,
					"size_bytes":   len(api.TestImageBytes),
				})
				Expect(setErr).NotTo(HaveOccurred())
				Expect(setResult.IsError).To(BeFalse())
				out := decodeSetImageOutput(setResult)

				putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
				Expect(putErr).NotTo(HaveOccurred())
				Expect(putResp.StatusCode).To(Equal(http.StatusOK))
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			})

			When("the delete_switch_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_switch_image", map[string]any{
						"switch_id": switchID,
					})
				})

				It("succeeds and has_image reflects it on a follow-up get_switch", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					check, checkErr := client.CallTool(ctx, "get_switch", map[string]any{"switch_id": switchID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
					Expect(decodeGetOutput(check).Switch.HasImage).To(BeFalse())
				})
			})
		})

		Context("given the caller owns the switch with no image set", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
			})

			When("the delete_switch_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_switch_image", map[string]any{
						"switch_id": switchID,
					})
				})

				It("succeeds idempotently", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
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

			When("the delete_switch_image tool is called with that switch_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_switch_image", map[string]any{
						"switch_id": switchID,
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the switch does not exist", func() {
			When("the delete_switch_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_switch_image", map[string]any{
						"switch_id": "no-such-switch-" + uuid.NewString(),
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

		When("the delete_switch_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_switch_image", map[string]any{
					"switch_id": "irrelevant-" + uuid.NewString(),
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
