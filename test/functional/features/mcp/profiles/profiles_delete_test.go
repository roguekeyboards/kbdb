package profiles_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a profile over MCP", func() {
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
				Username:     username,
				Discoverable: true,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			_ = db.DeleteProfile(ctx, ownerID, username)
		})

		When("delete_profile is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_profile", map[string]any{})
			})

			It("succeeds, the profile is gone, and the username is free again", func(ctx SpecContext) {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())

				By("no longer resolving the profile")
				got, getErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(got.IsError).To(BeTrue())

				By("letting another user claim the freed username")
				otherToken, otherID, subErr := api.NewAuthIdentity(ctx)
				Expect(subErr).NotTo(HaveOccurred())
				otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

				created, createErr := otherClient.CallTool(ctx, "create_profile", map[string]any{
					"username": username, "discoverable": true,
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(created.IsError).To(BeFalse())

				DeferCleanup(func(ctx SpecContext) {
					Expect(db.DeleteProfile(ctx, otherID, username)).To(Succeed())
				})
			})
		})
	})

	Context("given the caller has a profile and owns one of each item", func() {
		var keyboardID, switchID, keycapSetID, buildID string

		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:     username,
				Discoverable: true,
			})).To(Succeed())
			keyboardID, switchID, keycapSetID, buildID = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
			Expect(db.SeedKeyboardWithImage(ctx, ownerID, keyboardID, uuid.NewString(), "private")).To(Succeed())
			Expect(db.SeedSwitchWithImage(ctx, ownerID, switchID, "public")).To(Succeed())
			Expect(db.SeedKeycapSetWithKit(ctx, ownerID, keycapSetID, uuid.NewString(), "private")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			_ = db.DeleteProfile(ctx, ownerID, username)
			_ = db.DeleteBuild(ctx, ownerID, buildID, keyboardID)
			_ = db.DeleteKeyboard(ctx, ownerID, keyboardID)
			_ = db.DeleteSwitch(ctx, ownerID, switchID)
			_ = db.DeleteKeycapSet(ctx, ownerID, keycapSetID)
		})

		When("delete_profile is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_profile", map[string]any{})
			})

			It("succeeds and deletes every item", func(ctx SpecContext) {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())

				for tool, args := range map[string]map[string]any{
					"get_keyboard":   {"keyboard_id": keyboardID},
					"get_switch":     {"switch_id": switchID},
					"get_keycap_set": {"keycap_set_id": keycapSetID},
					"get_build":      {"build_id": buildID},
				} {
					got, getErr := client.CallTool(ctx, tool, args)
					Expect(getErr).NotTo(HaveOccurred())
					Expect(got.IsError).To(BeTrue(), tool)
				}
			})
		})
	})

	Context("given the caller has no profile", func() {
		When("delete_profile is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_profile", map[string]any{})
			})

			It("succeeds idempotently", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("delete_profile is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_profile", map[string]any{})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
