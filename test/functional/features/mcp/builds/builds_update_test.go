package builds_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Updating a build over MCP", func() {
	var (
		client     *api.MCPClient
		result     *sdkmcp.CallToolResult
		err        error
		ownerID    string
		keyboardID string
		buildID    string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		buildID = "functional-test-build-" + uuid.NewString()
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)

			keyboardID = "build-fixture-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given the caller owns the build", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			When("the update_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_build", map[string]any{
						"build_id":   buildID,
						"keyboard":   keyboardID,
						"visibility": "private",
						"notes":      "rebuilt",
					})
				})

				It("replaces the build's fields, persisted", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.ID).To(Equal(buildID))

					By("actually persisting the new notes, not a no-op")
					check, checkErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(decodeBuildOutput(check).Build.ID).To(Equal(buildID))
				})
			})

			Context("given a request changing visibility to public", func() {
				When("the update_build tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_build", map[string]any{
							"build_id":   buildID,
							"keyboard":   keyboardID,
							"visibility": "public",
						})
					})

					It("makes the build visible to another authenticated user", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						otherToken, _, tokenErr := api.NewAuthIdentity(ctx)
						Expect(tokenErr).NotTo(HaveOccurred())
						otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

						check, checkErr := otherClient.CallTool(ctx, "get_build", map[string]any{
							"build_id": buildID,
							"user_id":  ownerID,
						})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeBuildOutput(check).Build.ID).To(Equal(buildID))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("the update_build tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_build", map[string]any{
							"build_id":   buildID,
							"keyboard":   keyboardID,
							"visibility": "private",
							"stabs":      map[string]any{"name": "NotApproved"},
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given a required field is present but blank", func() {
				When("the update_build tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_build", map[string]any{
							"build_id":   buildID,
							"keyboard":   "",
							"visibility": "private",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the build references a keyboard that doesn't exist", func() {
				When("the update_build tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_build", map[string]any{
							"build_id":   buildID,
							"keyboard":   "does-not-exist-" + uuid.NewString(),
							"visibility": "private",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})
		})

		Context("given the build never existed", func() {
			When("the update_build tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_build", map[string]any{
						"build_id":   buildID,
						"keyboard":   keyboardID,
						"visibility": "private",
					})
				})

				It("returns an MCP tool error result, not idempotent like delete", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns the build", func() {
			var (
				otherID         string
				otherKeyboardID string
			)

			var otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
				Expect(db.SeedKeyboard(ctx, otherID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedBuild(ctx, otherID, buildID, otherKeyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteKeyboard(ctx, otherID, otherKeyboardID)).To(Succeed())
			})

			When("the update_build tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_build", map[string]any{
						"build_id":   buildID,
						"keyboard":   keyboardID,
						"visibility": "public",
					})
				})

				It("leaves the other user's build untouched", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
					Expect(decodeBuildOutput(check).Build.Keyboard).To(Equal(otherKeyboardID))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the update_build tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_build", map[string]any{
					"build_id":   buildID,
					"keyboard":   "does-not-matter",
					"visibility": "private",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
