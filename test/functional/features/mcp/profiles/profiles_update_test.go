package profiles_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Updating a profile over MCP", func() {
	var (
		client   *api.MCPClient
		result   *sdkmcp.CallToolResult
		err      error
		ownerID  string
		username string
	)

	BeforeEach(func(ctx SpecContext) {
		result = nil
		err = nil
		username = "u" + uuid.NewString()[:8]

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)
	})

	Context("given the caller has a profile", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:        username,
				Discoverable:    true,
				Bio:             "original bio",
				DiscordUsername: "alice_kb",
				Links: []map[string]string{
					{"name": "Twitch", "url": "https://twitch.tv/alice"},
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			_ = db.DeleteProfile(ctx, ownerID, username)
		})

		Context("given a full replace that changes the bio and omits links", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     username,
						"discoverable": true,
						"bio":          "brand new bio",
					})
				})

				It("changes the bio and clears the links", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetProfileOutput(result)
					Expect(out.Profile.Bio).NotTo(BeNil())
					Expect(*out.Profile.Bio).To(Equal("brand new bio"))
					Expect(out.Profile.Links).To(BeEmpty())

					read, readErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
					Expect(readErr).NotTo(HaveOccurred())
					Expect(decodeGetProfileOutput(read).Profile.Links).To(BeEmpty())
				})
			})
		})

		Context("given a rename to an unclaimed username", func() {
			var newUsername string

			BeforeEach(func() {
				newUsername = "u" + uuid.NewString()[:8]
			})

			AfterEach(func(ctx SpecContext) {
				_ = db.DeleteProfile(ctx, ownerID, newUsername)
			})

			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     newUsername,
						"discoverable": true,
					})
				})

				It("moves the username claim", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					byNew, readErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": newUsername})
					Expect(readErr).NotTo(HaveOccurred())
					Expect(byNew.IsError).To(BeFalse())

					byOld, readErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": username})
					Expect(readErr).NotTo(HaveOccurred())
					Expect(byOld.IsError).To(BeTrue())
				})
			})
		})

		Context("given the same username", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     username,
						"discoverable": true,
						"bio":          "no rename",
					})
				})

				It("succeeds with no self-conflict", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
				})
			})
		})

		Context("given a full replace that changes preferences", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     username,
						"discoverable": true,
						"preferences": map[string]any{
							"currency": "GBP", "show_price_to_me": false, "show_price_to_others": true,
						},
					})
				})

				It("changes the preferences", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					prefs := decodeGetProfileOutput(result).Profile.Preferences
					Expect(prefs.Currency).To(Equal("GBP"))
					Expect(prefs.ShowPriceToMe).To(BeFalse())
					Expect(prefs.ShowPriceToOthers).To(BeTrue())
				})
			})
		})

		Context("given a full replace that omits preferences", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     username,
						"discoverable": true,
					})
				})

				It("resets preferences to defaults", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					prefs := decodeGetProfileOutput(result).Profile.Preferences
					Expect(prefs.Currency).To(Equal("USD"))
					Expect(prefs.ShowPriceToMe).To(BeTrue())
					Expect(prefs.ShowPriceToOthers).To(BeFalse())
				})
			})
		})

		Context("given a blank discord_username", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":         username,
						"discoverable":     true,
						"discord_username": "   ",
					})
				})

				It("clears discord_username instead of leaving it empty", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeGetProfileOutput(result).Profile.DiscordUsername).To(BeNil())
				})
			})
		})

		Context("given a malformed discord_username", func() {
			When("update_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":         username,
						"discoverable":     true,
						"discord_username": "na@me",
					})
				})

				It("returns a tool error", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the new username is taken by another user", func() {
			var (
				otherID   string
				otherUser string
			)

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				otherUser = "u" + uuid.NewString()[:8]
				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: otherUser, Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, otherUser)).To(Succeed())
			})

			When("update_profile is called with that username", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_profile", map[string]any{
						"username":     otherUser,
						"discoverable": true,
					})
				})

				It("returns a tool error", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})

	Context("given the caller has no profile", func() {
		When("update_profile is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_profile", map[string]any{"username": username})
			})

			It("returns a tool error", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeTrue())
			})
		})
	})
})
