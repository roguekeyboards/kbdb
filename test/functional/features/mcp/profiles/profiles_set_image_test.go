package profiles_test

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

var _ = Describe("Setting a profile's avatar over MCP", func() {
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

		Context("given the caller has a profile", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
					Username:     username,
					Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
			})

			Context("given an approved content_type", func() {
				When("the set_profile_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_profile_image", map[string]any{
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("mints an upload_url that a real PUT round-trip works against, then has_avatar is true", func(ctx SpecContext) {
						By("returning an upload_url")
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
						out := decodeSetProfileImageOutput(result)
						Expect(out.UploadURL).NotTo(BeEmpty())

						By("uploading a PNG to the presigned PUT URL")
						imageBytes := api.TestImageBytes
						putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(imageBytes))
						Expect(putErr).NotTo(HaveOccurred())
						Expect(putResp.StatusCode).To(Equal(http.StatusOK))

						By("has_avatar reflecting it on a follow-up get_profile")
						check, checkErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeGetProfileOutput(check).Profile.HasAvatar).To(BeTrue())
					})
				})
			})

			Context("given an unapproved content_type", func() {
				When("the set_profile_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_profile_image", map[string]any{
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

			Context("given the avatar is already set", func() {
				BeforeEach(func(ctx SpecContext) {
					first, firstErr := client.CallTool(ctx, "set_profile_image", map[string]any{
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
					})
					Expect(firstErr).NotTo(HaveOccurred())
					Expect(first.IsError).To(BeFalse())
				})

				When("the set_profile_image tool is called again", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "set_profile_image", map[string]any{
							"content_type": approvedImageContentType,
							"size_bytes":   len(api.TestImageBytes),
						})
					})

					It("replaces it, no need to delete first", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
					})
				})
			})
		})

		Context("given the caller has no profile", func() {
			When("the set_profile_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "set_profile_image", map[string]any{
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

		When("the set_profile_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "set_profile_image", map[string]any{
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
