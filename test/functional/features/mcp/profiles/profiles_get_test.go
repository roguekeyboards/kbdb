package profiles_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Getting a profile over MCP", func() {
	var (
		client   *api.MCPClient
		result   *sdkmcp.CallToolResult
		err      error
		ownerID  string
		username string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		username = "u" + uuid.NewString()[:8]
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given a discoverable profile exists", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
					Username:        username,
					Discoverable:    true,
					DiscordUsername: "alice_kb",
					Bio:             "keebs",
					Links:           []map[string]string{{"name": "Twitch", "url": "https://twitch.tv/alice"}},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
			})

			Context("given the identifier is the user id", func() {
				When("get_profile is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
					})

					It("returns the full profile", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						out := decodeGetProfileOutput(result)
						Expect(out.Profile.Username).To(Equal(username))
						Expect(out.Profile.UserID).To(Equal(ownerID))
						Expect(out.Profile.Bio).NotTo(BeNil())
						Expect(*out.Profile.Bio).To(Equal("keebs"))
						Expect(out.Profile.Links).To(HaveLen(1))
						Expect(out.Profile.HasAvatar).To(BeFalse())
					})
				})
			})

			Context("given the identifier is the username", func() {
				When("get_profile is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "get_profile", map[string]any{"identifier": username})
					})

					It("resolves the username and returns the profile", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						Expect(decodeGetProfileOutput(result).Profile.Username).To(Equal(username))
					})
				})
			})
		})

		Context("given a non-discoverable profile owned by another user", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username:     username,
					Discoverable: false,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, username)).To(Succeed())
			})

			When("get_profile is called with that user's id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_profile", map[string]any{"identifier": otherID})
				})

				It("is indistinguishable from the profile not existing", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given a discoverable profile owned by another user", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username:     username,
					Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, username)).To(Succeed())
			})

			When("get_profile is called with that user's id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_profile", map[string]any{"identifier": otherID})
				})

				It("returns the profile without its preferences", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeGetProfileOutput(result)
					Expect(out.Profile.UserID).To(Equal(otherID))
					Expect(out.Profile.Preferences).To(BeNil())
				})
			})
		})

		Context("given no profile matches the identifier", func() {
			When("get_profile is called with a bogus username", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_profile", map[string]any{
						"identifier": "ghost" + uuid.NewString()[:8],
					})
				})

				It("returns a not-found tool error", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})
})
