package keyboards_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Getting a keyboard over MCP", func() {
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

			When("the get_keyboard tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
				})

				It("defaults to the caller's own collection and returns the keyboard", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Keyboard.ID).To(Equal(keyboardID))
					Expect(out.Keyboard.Brand).To(Equal("Keychron"))
					Expect(out.Keyboard.Visibility).NotTo(BeNil())
					Expect(*out.Keyboard.Visibility).To(Equal("private"))

					By("round-tripping the nested groups out of DynamoDB")
					Expect(out.Keyboard.Design).NotTo(BeNil())
					Expect(out.Keyboard.Design.TopCase).NotTo(BeNil())
					Expect(*out.Keyboard.Design.TopCase.Material).To(Equal("Aluminum"))
					Expect(out.Keyboard.Plates).To(HaveLen(1))
					Expect(out.Keyboard.Plates[0].ID).To(Equal(db.SeededPlateID))
					Expect(out.Keyboard.Plates[0].Material).To(Equal("FR4"))
					Expect(out.Keyboard.Plates[0].Purchase).NotTo(BeNil())
					Expect(*out.Keyboard.Plates[0].Purchase.Price).To(Equal(40.0))
					Expect(out.Keyboard.PCBs).To(HaveLen(1))
					Expect(out.Keyboard.PCBs[0].ID).To(Equal(db.SeededPCBID))
					Expect(*out.Keyboard.PCBs[0].Firmware).To(Equal("QMK/VIA"))
					Expect(out.Keyboard.TotalCost).NotTo(BeNil())
					Expect(*out.Keyboard.TotalCost).To(Equal(369.99))
					Expect(out.Keyboard.Purchase).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Vendor).To(Equal("Amazon"))
					Expect(out.Keyboard.Purchase.Price).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Price).To(Equal(329.99))

					By("omitting a group whose fields are all unset")
					Expect(out.Keyboard.Design.BottomCase).To(BeNil())
				})
			})
		})

		Context("given the caller owns the keyboard and has show_price_to_me false", func() {
			var profileUsername string

			BeforeEach(func(ctx SpecContext) {
				profileUsername = "u" + uuid.NewString()[:8]
				Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
				Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
			})

			When("the get_keyboard tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
				})

				It("still returns purchase.price - single-item get always shows the owner their own price", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Keyboard.Purchase).NotTo(BeNil())
					Expect(out.Keyboard.Purchase.Price).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Price).To(Equal(329.99))
				})
			})
		})

		Context("given the keyboard never existed", func() {
			When("the get_keyboard tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
				})

				It("returns an MCP tool error result, not a transport failure", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a private keyboard", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedKeyboard(ctx, otherID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, otherID, keyboardID)).To(Succeed())
			})

			When("the get_keyboard tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"user_id":     otherID,
					})
				})

				It("is indistinguishable from the keyboard not existing", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a public keyboard", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedKeyboard(ctx, otherID, keyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, otherID, keyboardID)).To(Succeed())
			})

			When("the get_keyboard tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"user_id":     otherID,
					})
				})

				It("returns the keyboard with purchase.price and visibility omitted", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Keyboard.ID).To(Equal(keyboardID))

					By("still including non-price purchase fields")
					Expect(out.Keyboard.Purchase).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Vendor).To(Equal("Amazon"))

					By("omitting price")
					Expect(out.Keyboard.Purchase.Price).To(BeNil())

					By("omitting visibility")
					Expect(out.Keyboard.Visibility).To(BeNil())
				})
			})
		})

		Context("given another user owns a public keyboard, has show_price_to_others true and a non-discoverable profile", func() {
			var (
				otherID         string
				profileUsername string
			)

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)
				profileUsername = "u" + uuid.NewString()[:8]

				Expect(db.SeedKeyboard(ctx, otherID, keyboardID, "public")).To(Succeed())
				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "EUR", "show_price_to_me": true, "show_price_to_others": true,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeyboard(ctx, otherID, keyboardID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, otherID, profileUsername)).To(Succeed())
			})

			When("the get_keyboard tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keyboard", map[string]any{
						"keyboard_id": keyboardID,
						"user_id":     otherID,
					})
				})

				It("returns the keyboard with purchase.price and the owner's currency", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetOutput(result)
					Expect(out.Keyboard.Purchase).NotTo(BeNil())
					Expect(out.Keyboard.Purchase.Price).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Price).To(Equal(329.99))

					By("including the owner's currency, which their non-discoverable profile can't provide")
					Expect(out.Keyboard.Purchase.Currency).NotTo(BeNil())
					Expect(*out.Keyboard.Purchase.Currency).To(Equal("EUR"))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the get_keyboard tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
