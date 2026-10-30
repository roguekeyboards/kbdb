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

var _ = Describe("Deleting a build over MCP", func() {
	var (
		client     *api.MCPClient
		result     *sdkmcp.CallToolResult
		err        error
		ownerID    string
		keyboardID string
		buildID    string
		deleted    bool
	)

	BeforeEach(func() {
		result = nil
		err = nil
		deleted = false
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
				if !deleted {
					Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
				}
			})

			When("the delete_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_build", map[string]any{"build_id": buildID})
					if err == nil && !result.IsError {
						deleted = true
					}
				})

				It("removes the build", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					By("a subsequent get_build no longer finding it")
					check, checkErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeTrue())
				})
			})
		})

		Context("given the build never existed", func() {
			When("the delete_build tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_build", map[string]any{"build_id": buildID})
				})

				It("succeeds, since deleting is idempotent", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
				})
			})
		})

		Context("given another user owns the build", func() {
			var otherID, otherKeyboardID, otherToken string

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

			When("the delete_build tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_build", map[string]any{"build_id": buildID})
				})

				It("leaves the other user's build in place", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the delete_build tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_build", map[string]any{"build_id": buildID})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
