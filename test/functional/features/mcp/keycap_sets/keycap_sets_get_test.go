package keycapsets_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Getting a keycap set over MCP", func() {
	var (
		client      *api.MCPClient
		result      *sdkmcp.CallToolResult
		err         error
		ownerID     string
		keycapSetID string
	)

	BeforeEach(func() {
		result = nil
		err = nil
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
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("defaults to the caller's own collection and returns the keycap set", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.ID).To(Equal(keycapSetID))
					Expect(out.KeycapSet.Brand).To(Equal("GMK"))
					Expect(out.KeycapSet.Visibility).To(Equal("private"))
				})
			})
		})

		Context("given the caller owns the keycap set and it has a kit", func() {
			var kitID string

			BeforeEach(func(ctx SpecContext) {
				kitID = "kit-" + uuid.NewString()
				Expect(db.SeedKeycapSetWithKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("includes the kit's purchase.price for the owner", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.Kits).To(HaveLen(1))
					Expect(out.KeycapSet.Kits[0].Purchase).NotTo(BeNil())
					Expect(out.KeycapSet.Kits[0].Purchase.Price).NotTo(BeNil())
					Expect(*out.KeycapSet.Kits[0].Purchase.Price).To(Equal(85.0))

					By("including total_cost and the owner's currency")
					Expect(out.KeycapSet.TotalCost).To(HaveValue(Equal(85.0)))
					Expect(out.KeycapSet.Currency).To(HaveValue(Equal("USD")))
				})
			})
		})

		Context("given the caller owns the keycap set with a kit and has show_price_to_me false", func() {
			var (
				kitID           string
				profileUsername string
			)

			BeforeEach(func(ctx SpecContext) {
				kitID = "kit-" + uuid.NewString()
				profileUsername = "u" + uuid.NewString()[:8]
				Expect(db.SeedKeycapSetWithKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
				Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
			})

			When("the get_keycap_set tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("still returns the kit's purchase.price - single-item get always shows the owner their own price", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.Kits).To(HaveLen(1))
					Expect(out.KeycapSet.Kits[0].Purchase).NotTo(BeNil())
					Expect(out.KeycapSet.Kits[0].Purchase.Price).NotTo(BeNil())
					Expect(*out.KeycapSet.Kits[0].Purchase.Price).To(Equal(85.0))
				})
			})
		})

		Context("given the caller owns the keycap set and it has a primary kit", func() {
			var kitID string

			BeforeEach(func(ctx SpecContext) {
				kitID = "kit-" + uuid.NewString()
				Expect(db.SeedKeycapSetWithPrimaryKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("returns primary_kit_id matching the seeded kit", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.PrimaryKitID).NotTo(BeNil())
					Expect(*out.KeycapSet.PrimaryKitID).To(Equal(kitID))
				})
			})
		})

		Context("given the caller owns the keycap set and its primary kit no longer exists", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedKeycapSetWithDanglingPrimaryKit(ctx, ownerID, keycapSetID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("returns a nil primary_kit_id", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.PrimaryKitID).To(BeNil())
				})
			})
		})

		Context("given the keycap set never existed", func() {
			When("the get_keycap_set tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
				})

				It("returns an MCP tool error result, not a transport failure", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a private keycap set", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"user_id":       otherID,
					})
				})

				It("is indistinguishable from the keycap set not existing", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a public keycap set", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"user_id":       otherID,
					})
				})

				It("returns the keycap set", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeKeycapSetOutput(result).KeycapSet.ID).To(Equal(keycapSetID))
				})
			})
		})

		Context("given another user owns a public keycap set with a kit", func() {
			var (
				otherID string
				kitID   string
			)

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				kitID = "kit-" + uuid.NewString()
				Expect(db.SeedKeycapSetWithKit(ctx, otherID, keycapSetID, kitID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the get_keycap_set tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"user_id":       otherID,
					})
				})

				It("returns the kit with purchase.price omitted", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.Kits).To(HaveLen(1))

					By("still including non-price purchase fields")
					Expect(out.KeycapSet.Kits[0].Purchase).NotTo(BeNil())
					Expect(*out.KeycapSet.Kits[0].Purchase.Vendor).To(Equal("MechMarket"))

					By("omitting price")
					Expect(out.KeycapSet.Kits[0].Purchase.Price).To(BeNil())
				})
			})
		})

		Context("given another user owns a public keycap set with a kit and has show_price_to_others true", func() {
			var (
				otherID         string
				kitID           string
				profileUsername string
			)

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				kitID = "kit-" + uuid.NewString()
				profileUsername = "u" + uuid.NewString()[:8]
				Expect(db.SeedKeycapSetWithKit(ctx, otherID, keycapSetID, kitID, "public")).To(Succeed())
				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "USD", "show_price_to_me": true, "show_price_to_others": true,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, otherID, profileUsername)).To(Succeed())
			})

			When("the get_keycap_set tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"user_id":       otherID,
					})
				})

				It("returns the kit with purchase.price", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.Kits).To(HaveLen(1))
					Expect(out.KeycapSet.Kits[0].Purchase).NotTo(BeNil())
					Expect(out.KeycapSet.Kits[0].Purchase.Price).NotTo(BeNil())
					Expect(*out.KeycapSet.Kits[0].Purchase.Price).To(Equal(85.0))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the get_keycap_set tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
