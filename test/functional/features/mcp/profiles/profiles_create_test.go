package profiles_test

import (
	"strings"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Creating a profile over MCP", func() {
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

	Context("given the caller has no profile", func() {
		AfterEach(func(ctx SpecContext) {
			_ = db.DeleteProfile(ctx, ownerID, username)
		})

		Context("given valid input", func() {
			When("create_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{
						"username":     username,
						"discoverable": true,
						"bio":          "keebs",
					})
				})

				It("creates the profile and it is then readable", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeGetProfileOutput(result).Profile.Username).To(Equal(username))

					read, readErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
					Expect(readErr).NotTo(HaveOccurred())
					Expect(read.IsError).To(BeFalse())
					Expect(decodeGetProfileOutput(read).Profile.Username).To(Equal(username))
				})
			})
		})

		Context("given a username containing a period and a hyphen", func() {
			BeforeEach(func() {
				username = "my.kb-" + uuid.NewString()[:8]
			})

			When("create_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{
						"username":     username,
						"discoverable": true,
					})
				})

				It("creates the profile and it is then readable by that username", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					read, readErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": username})
					Expect(readErr).NotTo(HaveOccurred())
					Expect(read.IsError).To(BeFalse())
					Expect(decodeGetProfileOutput(read).Profile.Username).To(Equal(username))
				})
			})
		})

		Context("given preferences are omitted", func() {
			When("create_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{
						"username": username, "discoverable": true,
					})
				})

				It("creates the profile with default preferences", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					prefs := decodeGetProfileOutput(result).Profile.Preferences
					Expect(prefs.Currency).To(Equal("USD"))
					Expect(prefs.ShowPriceToMe).To(BeTrue())
					Expect(prefs.ShowPriceToOthers).To(BeFalse())
				})
			})
		})

		Context("given explicit preferences", func() {
			When("create_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{
						"username": username, "discoverable": true,
						"preferences": map[string]any{
							"currency": "EUR", "show_price_to_me": false, "show_price_to_others": true,
						},
					})
				})

				It("creates the profile with the given preferences", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					prefs := decodeGetProfileOutput(result).Profile.Preferences
					Expect(prefs.Currency).To(Equal("EUR"))
					Expect(prefs.ShowPriceToMe).To(BeFalse())
					Expect(prefs.ShowPriceToOthers).To(BeTrue())
				})
			})
		})

		Context("given a blank discord_username", func() {
			When("create_profile is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{
						"username":         username,
						"discoverable":     true,
						"discord_username": "   ",
					})
				})

				It("creates the profile with discord_username absent, not empty", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeGetProfileOutput(result).Profile.DiscordUsername).To(BeNil())
				})
			})
		})

		Context("given a username already claimed by another user", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: username, Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, username)).To(Succeed())
			})

			When("create_profile is called with that username", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_profile", map[string]any{"username": username})
				})

				It("returns a tool error naming the taken username", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		DescribeTable("given the input fails validation",
			func(ctx SpecContext, args map[string]any) {
				result, err = client.CallTool(ctx, "create_profile", args)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeTrue())
			},
			Entry("username fails the pattern", map[string]any{"username": "AB"}),
			Entry("username has a leading period", map[string]any{"username": ".lead"}),
			Entry("username has a trailing underscore", map[string]any{"username": "trail_"}),
			Entry("username has consecutive periods", map[string]any{"username": "a..b"}),
			Entry("username is over 32 characters", map[string]any{"username": strings.Repeat("x", 33)}),
			Entry("a link url is http not https", map[string]any{
				"username": "aaa",
				"links":    []map[string]any{{"name": "s", "url": "http://x.example"}},
			}),
			Entry("discord_username over 32", map[string]any{
				"username": "aaa", "discord_username": strings.Repeat("x", 33),
			}),
			Entry("discord_username has an invalid character", map[string]any{
				"username": "aaa", "discord_username": "na@me",
			}),
			Entry("bio over 500", map[string]any{
				"username": "aaa", "bio": strings.Repeat("x", 501),
			}),
			Entry("currency is not 3 letters", map[string]any{
				"username": "aaa", "preferences": map[string]any{"currency": "US"},
			}),
		)
	})

	Context("given the caller already has a profile", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: username, Discoverable: false,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		When("create_profile is called again", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "create_profile", map[string]any{
					"username": "other" + uuid.NewString()[:6],
				})
			})

			It("returns a tool error", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeTrue())
			})
		})
	})
})
