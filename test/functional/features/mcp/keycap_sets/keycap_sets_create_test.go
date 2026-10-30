package keycapsets_test

import (
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Creating a keycap set over MCP", func() {
	var (
		client    *api.MCPClient
		result    *sdkmcp.CallToolResult
		err       error
		ownerID   string
		createdID string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		createdID = ""
	})

	captureCreatedID := func(r *sdkmcp.CallToolResult) {
		if r == nil || r.IsError {
			return
		}
		createdID = decodeKeycapSetOutput(r).KeycapSet.ID
	}

	AfterEach(func(ctx SpecContext) {
		if createdID != "" {
			Expect(db.DeleteKeycapSet(ctx, ownerID, createdID)).To(Succeed())
		}
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given valid arguments", func() {
			When("the create_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
						"brand":      "GMK",
						"name":       "Laser",
						"visibility": "private",
					})
					captureCreatedID(result)
				})

				It("creates the keycap set in the caller's own collection", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.ID).NotTo(BeEmpty(), "the server assigns the id")
					Expect(out.KeycapSet.Brand).To(Equal("GMK"))
					Expect(out.KeycapSet.Visibility).To(Equal("private"))
				})
			})
		})

		Context("given every open-vocabulary field has an approved lookup value", func() {
			When("the create_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
						"brand":      "GMK",
						"name":       "Laser",
						"visibility": "private",
						"profile":    approvedProfile,
						"material":   approvedMaterial,
					})
					captureCreatedID(result)
				})

				It("creates the keycap set", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeKeycapSetOutput(result).KeycapSet.ID).NotTo(BeEmpty())
				})
			})
		})

		Context("given an open-vocabulary field has an unapproved value", func() {
			When("the create_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
						"brand":      "GMK",
						"name":       "Laser",
						"visibility": "private",
						"profile":    "NotApproved",
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given a required field is present but blank", func() {
			When("the create_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
						"brand":      "",
						"name":       "Laser",
						"visibility": "private",
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given an invalid visibility", func() {
			When("the create_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
						"brand":      "GMK",
						"name":       "Laser",
						"visibility": "everyone",
					})
					captureCreatedID(result)
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

		When("the create_keycap_set tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "create_keycap_set", map[string]any{
					"brand":      "GMK",
					"name":       "Laser",
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
