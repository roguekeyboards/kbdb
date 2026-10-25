package builds_test

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

var _ = Describe("Listing a build's images over MCP", func() {
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

		Context("given the caller owns a build with no images", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			When("the list_build_images tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_build_images", map[string]any{"build_id": buildID})
				})

				It("succeeds with an empty images list", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeListImagesOutput(result).Images).To(BeEmpty())
				})
			})
		})

		Context("given the caller owns a build with an image on it", func() {
			var imageID string

			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())

				addResult, addErr := client.CallTool(ctx, "add_build_image", map[string]any{
					"build_id":     buildID,
					"content_type": approvedImageContentType,
					"size_bytes":   len(api.TestImageBytes),
				})
				Expect(addErr).NotTo(HaveOccurred())
				Expect(addResult.IsError).To(BeFalse())
				out := decodeAddImageOutput(addResult)
				imageID = out.ImageID

				putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
				Expect(putErr).NotTo(HaveOccurred())
				Expect(putResp.StatusCode).To(Equal(http.StatusOK))
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			When("the list_build_images tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_build_images", map[string]any{"build_id": buildID})
				})

				It("reports the image_id", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(imageIDsOf(decodeListImagesOutput(result))).To(ConsistOf(imageID))
				})
			})
		})

		Context("given another user owns the build", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				otherID = api.NewOtherUserID(ctx)

				Expect(db.SeedBuild(ctx, otherID, buildID, keyboardID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, otherID, buildID, keyboardID)).To(Succeed())
			})

			When("the list_build_images tool is called with that build_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_build_images", map[string]any{"build_id": buildID})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the build does not exist", func() {
			When("the list_build_images tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "list_build_images", map[string]any{
						"build_id": "no-such-build-" + uuid.NewString(),
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the list_build_images tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "list_build_images", map[string]any{
					"build_id": "irrelevant-" + uuid.NewString(),
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
