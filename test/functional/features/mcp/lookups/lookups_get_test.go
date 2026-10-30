package lookups_test

import (
	"encoding/json"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
)

var _ = Describe("Getting a lookup category", func() {
	var (
		client *api.MCPClient
		result *sdkmcp.CallToolResult
		err    error
	)

	BeforeEach(func() {
		result = nil
		err = nil
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			token, _, tokenErr := api.NewAuthIdentity(ctx)
			Expect(tokenErr).NotTo(HaveOccurred())
			client = api.NewMCPClient(support.BaseURL()+"/mcp", token)
		})

		Context("given the category exists", func() {
			When("the get_lookup tool is called with that category", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_lookup", map[string]any{"category": "vendor"})
				})

				It("succeeds and returns the category's values", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					raw, err := json.Marshal(result.StructuredContent)
					Expect(err).NotTo(HaveOccurred())

					var out struct {
						Category string   `json:"category"`
						Values   []string `json:"values"`
					}
					Expect(json.Unmarshal(raw, &out)).To(Succeed())
					Expect(out.Category).To(Equal("vendor"))
					Expect(out.Values).To(ContainElement("Amazon"))
				})
			})
		})

		Context("given the category does not exist", func() {
			When("the get_lookup tool is called with that category", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "get_lookup", map[string]any{"category": "functional-test-category-missing-" + uuid.NewString()})
				})

				It("returns an MCP tool error result, not a transport failure", func() {
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

		When("the get_lookup tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "get_lookup", map[string]any{"category": "vendor"})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
