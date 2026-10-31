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

var _ = Describe("Deleting a profile's avatar over MCP", func() {
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

			Context("given the profile has an avatar", func() {
				BeforeEach(func(ctx SpecContext) {
					setResult, setErr := client.CallTool(ctx, "set_profile_image", map[string]any{
						"content_type": approvedImageContentType,
						"size_bytes":   len(api.TestImageBytes),
					})
					Expect(setErr).NotTo(HaveOccurred())
					Expect(setResult.IsError).To(BeFalse())
					out := decodeSetProfileImageOutput(setResult)

					putResp, putErr := api.DoBlob(ctx, http.MethodPut, out.UploadURL, approvedImageContentType, bytes.NewReader(api.TestImageBytes))
					Expect(putErr).NotTo(HaveOccurred())
					Expect(putResp.StatusCode).To(Equal(http.StatusOK))
				})

				When("the delete_profile_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_profile_image", map[string]any{})
					})

					It("succeeds and has_avatar reflects it on a follow-up get_profile", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_profile", map[string]any{"identifier": ownerID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeGetProfileOutput(check).Profile.HasAvatar).To(BeFalse())
					})
				})
			})

			Context("given the profile has no avatar set", func() {
				When("the delete_profile_image tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_profile_image", map[string]any{})
					})

					It("succeeds idempotently", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())
					})
				})
			})
		})

		Context("given the caller has no profile", func() {
			When("the delete_profile_image tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "delete_profile_image", map[string]any{})
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

		When("the delete_profile_image tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "delete_profile_image", map[string]any{})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
