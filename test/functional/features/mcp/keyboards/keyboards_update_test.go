package keyboards_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Updating a keyboard over MCP", func() {
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

			When("the update_keyboard tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"brand":       "Cherry",
						"name":        "MX Board",
						"visibility":  "public",
					})
				})

				It("replaces the keyboard's fields", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Keyboard.ID).To(Equal(keyboardID))
					Expect(out.Keyboard.Brand).To(Equal("Cherry"))
					Expect(out.Keyboard.Visibility).NotTo(BeNil())
					Expect(*out.Keyboard.Visibility).To(Equal("public"))

					By("clearing an optional field that was omitted")
					Expect(out.Keyboard.Design).To(BeNil())
				})
			})
		})

		Context("given the caller owns a keyboard with a plate and PCB", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
			})

			Context("given a valid update", func() {
				When("the update_keyboard tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keyboard", map[string]any{
							"keyboard_id": keyboardID,
							"brand":       "Keychron",
							"name":        "Q1 Pro",
							"visibility":  "private",
						})
					})

					It("leaves the keyboard's plate and PCB as they were", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						out := decodeGetOutput(result)
						Expect(out.Keyboard.Plates).To(ConsistOf(HaveField("ID", db.SeededPlateID)))
						Expect(out.Keyboard.PCBs).To(ConsistOf(HaveField("ID", db.SeededPCBID)))
					})
				})
			})
		})

		Context("given the keyboard never existed", func() {
			When("the update_keyboard tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"brand":       "Cherry",
						"name":        "MX Board",
						"visibility":  "public",
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns the keyboard", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(db.SeedKeyboard(ctx, otherID, keyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, otherID, keyboardID)).To(Succeed())
			})

			When("the update_keyboard tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"brand":       "Hijacked",
						"name":        "Hijacked",
						"visibility":  "public",
					})
				})

				It("leaves the other user's keyboard untouched", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					By("the other user's keyboard still having its original brand")
					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_keyboard", map[string]any{
						"keyboard_id": keyboardID,
					})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
					Expect(decodeGetOutput(check).Keyboard.Brand).To(Equal("Keychron"))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the update_keyboard tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_keyboard", map[string]any{
					"keyboard_id": keyboardID,
					"brand":       "Cherry",
					"name":        "MX Board",
					"visibility":  "public",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
