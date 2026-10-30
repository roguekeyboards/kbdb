package profiles_test

import (
	"strings"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Listing profiles over MCP", func() {
	var (
		client  *api.MCPClient
		result  *sdkmcp.CallToolResult
		err     error
		ownerID string
	)

	BeforeEach(func(ctx SpecContext) {
		result = nil
		err = nil

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)
	})

	Context("given a discoverable and a non-discoverable profile exist", func() {
		// Constant-PK directory GSIs: filter by a unique prefix, assert
		// membership not counts.
		var discoverableName, hiddenName string

		BeforeEach(func(ctx SpecContext) {
			discoverableName = "mdisc" + strings.ToLower(uuid.NewString()[:12])
			hiddenName = "mhidden" + strings.ToLower(uuid.NewString()[:10])

			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:        discoverableName,
				Discoverable:    true,
				DiscordUsername: "mixed_case",
				Bio:             "should not appear in the directory row",
				Links:           []map[string]string{{"name": "Site", "url": "https://example.com"}},
			})).To(Succeed())
			DeferCleanup(func(cleanupCtx SpecContext) {
				Expect(db.DeleteProfile(cleanupCtx, ownerID, discoverableName)).To(Succeed())
			})

			otherID := api.NewOtherUserID(ctx)
			Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
				Username:     hiddenName,
				Discoverable: false,
			})).To(Succeed())
			DeferCleanup(func(cleanupCtx SpecContext) {
				Expect(db.DeleteProfile(cleanupCtx, otherID, hiddenName)).To(Succeed())
			})
		})

		Context("given no filter", func() {
			When("list_profiles is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_profiles", map[string]any{})
				})

				It("includes the discoverable profile but not the non-discoverable one, with no bio or links", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeListProfilesOutput(result)
					names := listProfileUsernames(out)
					Expect(names).To(ContainElement(discoverableName))
					Expect(names).NotTo(ContainElement(hiddenName))

					for _, p := range out.Profiles {
						if p.Username == discoverableName {
							Expect(p.UserID).To(Equal(ownerID))
							Expect(p.Bio).To(BeNil())
							Expect(p.Links).To(BeEmpty())
						}
					}
				})
			})
		})

		Context("given a username prefix filter", func() {
			Context("given the prefix is given verbatim", func() {
				When("list_profiles is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "list_profiles", map[string]any{"username": discoverableName})
					})

					It("returns only the matching discoverable profile", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						Expect(listProfileUsernames(decodeListProfilesOutput(result))).To(ConsistOf(discoverableName))
					})
				})
			})

			Context("given the prefix is uppercased", func() {
				When("list_profiles is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "list_profiles", map[string]any{
							"username": strings.ToUpper(discoverableName),
						})
					})

					It("still matches the all-lowercase stored username", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						Expect(listProfileUsernames(decodeListProfilesOutput(result))).To(ConsistOf(discoverableName))
					})
				})
			})
		})

		Context("given a discord_username prefix filter", func() {
			When("list_profiles is called with an uppercased prefix", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_profiles", map[string]any{"discord_username": "MIXED_C"})
				})

				It("matches via the discord index", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(listProfileUsernames(decodeListProfilesOutput(result))).To(ContainElement(discoverableName))
				})
			})
		})
	})

	Context("given both prefix filters are supplied", func() {
		When("list_profiles is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "list_profiles", map[string]any{
					"username": "a", "discord_username": "b",
				})
			})

			It("returns a tool error", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeTrue())
			})
		})
	})

	Context("given more discoverable profiles than the page limit share a username prefix", func() {
		var prefix string
		var names []string

		BeforeEach(func(ctx SpecContext) {
			prefix = "mpage" + strings.ToLower(uuid.NewString()[:10])
			names = []string{prefix + "a", prefix + "b", prefix + "c"}

			otherID := api.NewOtherUserID(ctx)

			owners := []string{ownerID, otherID, "user-mpage-" + uuid.NewString()}
			for i, name := range names {
				Expect(db.SeedProfile(ctx, owners[i], db.SeedProfileOptions{
					Username:     name,
					Discoverable: true,
				})).To(Succeed())
				owner, uname := owners[i], name
				DeferCleanup(func(cleanupCtx SpecContext) {
					Expect(db.DeleteProfile(cleanupCtx, owner, uname)).To(Succeed())
				})
			}
		})

		When("paging through the prefix with limit 2", func() {
			It("round-trips next_cursor to return the remaining profiles", func(ctx SpecContext) {
				first, firstErr := client.CallTool(ctx, "list_profiles", map[string]any{"username": prefix, "limit": 2})
				Expect(firstErr).NotTo(HaveOccurred())
				Expect(first.IsError).To(BeFalse())

				page1 := decodeListProfilesOutput(first)
				Expect(page1.Profiles).To(HaveLen(2))
				Expect(page1.NextCursor).NotTo(BeEmpty())

				second, secondErr := client.CallTool(ctx, "list_profiles", map[string]any{
					"username": prefix, "limit": 2, "cursor": page1.NextCursor,
				})
				Expect(secondErr).NotTo(HaveOccurred())
				Expect(second.IsError).To(BeFalse())

				page2 := decodeListProfilesOutput(second)
				all := append(listProfileUsernames(page1), listProfileUsernames(page2)...)
				Expect(all).To(ConsistOf(names))
			})
		})

		Context("given a next_cursor minted under the prefix", func() {
			var cursor string

			BeforeEach(func(ctx SpecContext) {
				first, err := client.CallTool(ctx, "list_profiles", map[string]any{"username": prefix, "limit": 2})
				Expect(err).NotTo(HaveOccurred())
				Expect(first.IsError).To(BeFalse())
				page1 := decodeListProfilesOutput(first)
				Expect(page1.NextCursor).NotTo(BeEmpty())
				cursor = page1.NextCursor
			})

			When("reusing it with a narrower prefix on the same index", func() {
				It("returns a tool error", func(ctx SpecContext) {
					narrower, err := client.CallTool(ctx, "list_profiles", map[string]any{
						"username": prefix + "a", "limit": 2, "cursor": cursor,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(narrower.IsError).To(BeTrue())
				})
			})

			When("reusing it with a different filter", func() {
				It("returns a tool error", func(ctx SpecContext) {
					differentFilter, err := client.CallTool(ctx, "list_profiles", map[string]any{
						"discord_username": "x", "limit": 2, "cursor": cursor,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(differentFilter.IsError).To(BeTrue())
				})
			})
		})
	})
})
