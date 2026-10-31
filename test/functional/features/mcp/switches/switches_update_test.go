package switches_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Updating a switch over MCP", func() {
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

			When("the update_switch tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_switch", map[string]any{
						"switch_id":  switchID,
						"brand":      "Cherry",
						"name":       "MX Black",
						"type":       approvedType,
						"visibility": "public",
					})
				})

				It("replaces the switch's fields", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Switch.ID).To(Equal(switchID))
					Expect(out.Switch.Brand).To(Equal("Cherry"))
					Expect(out.Switch.Visibility).To(Equal("public"))
				})
			})
		})

		Context("given the switch never existed", func() {
			When("the update_switch tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_switch", map[string]any{
						"switch_id":  switchID,
						"brand":      "Cherry",
						"name":       "MX Black",
						"type":       approvedType,
						"visibility": "public",
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		// The switch exists, but belongs to someone else. Writes read the
		// owner from the token, never from an argument, so this must not
		// touch the other user's item.
		Context("given another user owns the switch", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(db.SeedSwitch(ctx, otherID, switchID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, otherID, switchID)).To(Succeed())
			})

			When("the update_switch tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_switch", map[string]any{
						"switch_id":  switchID,
						"brand":      "Hijacked",
						"name":       "Hijacked",
						"type":       approvedType,
						"visibility": "public",
					})
				})

				It("leaves the other user's switch untouched", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					By("the other user's switch still having its original brand")
					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_switch", map[string]any{
						"switch_id": switchID,
					})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
					Expect(decodeGetOutput(check).Switch.Brand).To(Equal("Gateron"))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the update_switch tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_switch", map[string]any{
					"switch_id":  switchID,
					"brand":      "Cherry",
					"name":       "MX Black",
					"type":       approvedType,
					"visibility": "public",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
