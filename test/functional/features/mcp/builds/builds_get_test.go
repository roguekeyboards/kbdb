package builds_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Getting a build over MCP", func() {
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

			When("the get_build tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				})

				It("defaults to the caller's own collection and returns the build", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.ID).To(Equal(buildID))
					Expect(out.Build.Keyboard).To(Equal(keyboardID))
					Expect(out.Build.Visibility).To(Equal("private"))
				})
			})
		})

		Context("given the caller owns the build and it has stabs", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			When("the get_build tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				})

				It("includes stabs.price for the owner", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.Stabs).NotTo(BeNil())
					Expect(out.Build.Stabs.Price).NotTo(BeNil())
					Expect(*out.Build.Stabs.Price).To(Equal(12.5))
				})
			})
		})

		Context("given the caller owns the build with stabs and has show_price_to_me false", func() {
			var profileUsername string

			BeforeEach(func(ctx SpecContext) {
				profileUsername = "u" + uuid.NewString()[:8]
				Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
				Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
			})

			When("the get_build tool is called with no user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				})

				It("still returns stabs.price - single-item get always shows the owner their own price", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.Stabs).NotTo(BeNil())
					Expect(out.Build.Stabs.Price).NotTo(BeNil())
					Expect(*out.Build.Stabs.Price).To(Equal(12.5))
				})
			})
		})

		Context("given the build never existed", func() {
			When("the get_build tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				})

				It("returns an MCP tool error result, not a transport failure", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a private build", func() {
			var otherID, otherKeyboardID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
				Expect(db.SeedKeyboard(ctx, otherID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedBuild(ctx, otherID, buildID, otherKeyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteKeyboard(ctx, otherID, otherKeyboardID)).To(Succeed())
			})

			When("the get_build tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{
						"build_id": buildID,
						"user_id":  otherID,
					})
				})

				It("is indistinguishable from the build not existing", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns a public build", func() {
			var otherID, otherKeyboardID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
				Expect(db.SeedKeyboard(ctx, otherID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedBuild(ctx, otherID, buildID, otherKeyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteKeyboard(ctx, otherID, otherKeyboardID)).To(Succeed())
			})

			When("the get_build tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{
						"build_id": buildID,
						"user_id":  otherID,
					})
				})

				It("returns the build", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeBuildOutput(result).Build.ID).To(Equal(buildID))
				})
			})
		})

		Context("given another user owns a public build with stabs", func() {
			var otherID, otherKeyboardID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
				Expect(db.SeedKeyboard(ctx, otherID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedBuildWithStabs(ctx, otherID, buildID, otherKeyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteKeyboard(ctx, otherID, otherKeyboardID)).To(Succeed())
			})

			When("the get_build tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{
						"build_id": buildID,
						"user_id":  otherID,
					})
				})

				It("returns stabs with price omitted", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.Stabs).NotTo(BeNil())

					By("still including non-price stabs fields")
					Expect(out.Build.Stabs.Name).NotTo(BeNil())
					Expect(*out.Build.Stabs.Name).To(Equal("Durock v3"))

					By("omitting price")
					Expect(out.Build.Stabs.Price).To(BeNil())
				})
			})
		})

		Context("given another user owns a public build with stabs and has show_price_to_others true", func() {
			var otherID, otherKeyboardID, profileUsername string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)
				profileUsername = "u" + uuid.NewString()[:8]

				otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
				Expect(db.SeedKeyboard(ctx, otherID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedBuildWithStabs(ctx, otherID, buildID, otherKeyboardID, "public")).To(Succeed())
				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: profileUsername,
					Preferences: map[string]any{
						"currency": "USD", "show_price_to_me": true, "show_price_to_others": true,
					},
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteKeyboard(ctx, otherID, otherKeyboardID)).To(Succeed())
				Expect(db.DeleteProfile(ctx, otherID, profileUsername)).To(Succeed())
			})

			When("the get_build tool is called with that user_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_build", map[string]any{
						"build_id": buildID,
						"user_id":  otherID,
					})
				})

				It("returns stabs with price included", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.Stabs).NotTo(BeNil())
					Expect(out.Build.Stabs.Price).NotTo(BeNil())
					Expect(*out.Build.Stabs.Price).To(Equal(12.5))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the get_build tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
