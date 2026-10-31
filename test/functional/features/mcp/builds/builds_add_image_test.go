package builds_test

import (
	"bytes"
	"net/http"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

const approvedImageContentType = "image/webp"

var _ = Describe("Adding an image to a build over MCP", func() {
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

			Context("given an approved content_type", func() {
				When("the add_build_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_build_image", map[string]any{
							"build_id":     buildID,
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("mints an image_id and upload_url that a real PUT round-trip actually works against", func(ctx SpecContext) {
						By("returning an image_id and upload_url")
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						out := decodeAddImageOutput(result)
						Expect(out.ImageID).NotTo(BeEmpty())
						Expect(out.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(putErr).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("has_images reflecting it on a follow-up get_build")
						check, checkErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeBuildOutput(check).Build.HasImages).To(BeTrue())
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("the add_build_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "add_build_image", map[string]any{
							"build_id":     buildID,
							"content_type": "application/x-not-an-image",
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
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

			When("the add_build_image tool is called with that build_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "add_build_image", map[string]any{
						"build_id":     buildID,
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the build does not exist", func() {
			When("the add_build_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "add_build_image", map[string]any{
						"build_id":     "no-such-build-" + uuid.NewString(),
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
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

		When("the add_build_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "add_build_image", map[string]any{
					"build_id":     "irrelevant-" + uuid.NewString(),
					"content_type": approvedImageContentType,
					"size_bytes":   len(api.TestImageBytes),
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
