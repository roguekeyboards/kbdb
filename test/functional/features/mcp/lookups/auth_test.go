package lookups_test

import (
	"encoding/json"
	"net/http"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
)

// list_lookups is the vehicle here only because it needs no fixtures - the
// property under test is the transport's auth, which is shared by every
// tool.
var _ = Describe("Calling an MCP tool with a bad token", func() {
	var (
		client *api.MCPClient
		result *sdkmcp.CallToolResult
		err    error
	)

	BeforeEach(func() {
		result = nil
		err = nil
	})

	Context("given a malformed bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "not-a-valid-jwt")
		})

		When("a tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "list_lookups", map[string]any{})
			})

			It("rejects the call with a real HTTP 401, not silently falling back to anonymous", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(http.StatusText(http.StatusUnauthorized)))
			})
		})
	})
})

// Uses the raw HTTP client, not api.MCPClient - the MCP SDK's transport
// collapses a non-2xx response into an opaque error string, which can't
// assert on the actual status code or WWW-Authenticate header. Per the MCP
// spec, this header is how an unauthenticated client is supposed to
// discover the RFC 9728 metadata URL from the 401 itself; API Gateway's
// native authorizer can't add it (it rejects before Lambda runs), which is
// why /mcp verifies auth in-process instead (see middleware.RequireAuth).
var _ = Describe("Calling /mcp directly over HTTP with no valid auth", func() {
	var (
		client *api.Client
		resp   *http.Response
		err    error
	)

	BeforeEach(func() {
		client = api.NewClient()
		resp = nil
		err = nil
	})

	wwwAuthenticateNamesTheMetadataURL := func() {
		GinkgoHelper()
		Expect(resp).NotTo(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		Expect(wwwAuth).To(HavePrefix("Bearer "))
		Expect(wwwAuth).To(ContainSubstring(`resource_metadata="`))
		Expect(wwwAuth).To(ContainSubstring("/.well-known/oauth-protected-resource/mcp"))
	}

	Context("given no Authorization header at all", func() {
		When("an MCP request is sent", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodPost, "/mcp", "",
					strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
			})

			It("rejects with a 401 whose WWW-Authenticate header names the metadata URL", func() {
				Expect(err).NotTo(HaveOccurred())
				wwwAuthenticateNamesTheMetadataURL()
			})
		})
	})

	Context("given an invalid bearer token", func() {
		When("an MCP request is sent", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodPost, "/mcp", "not-a-valid-jwt",
					strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
			})

			It("rejects with a 401 whose WWW-Authenticate header names the metadata URL", func() {
				Expect(err).NotTo(HaveOccurred())
				wwwAuthenticateNamesTheMetadataURL()
			})
		})
	})
})

// The RFC 9728 Protected Resource Metadata document must itself be
// reachable with no auth - it exists so an unauthenticated client can
// discover where to authenticate. Gating it behind auth (a real bug this
// suite would have caught) is self-defeating.
var _ = Describe("Fetching MCP's OAuth Protected Resource Metadata", func() {
	var (
		client *api.Client
		resp   *http.Response
		err    error
		body   map[string]any
	)

	BeforeEach(func() {
		client = api.NewClient()
		resp = nil
		err = nil
		body = nil
	})

	fetchesSuccessfully := func() {
		GinkgoHelper()
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body).To(HaveKey("resource"))
		Expect(body).To(HaveKey("authorization_servers"))
	}

	Context("given no Authorization header", func() {
		When("the root well-known path is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/.well-known/oauth-protected-resource", "", nil)
			})

			It("returns the metadata document, not a 401", func() {
				fetchesSuccessfully()
			})
		})

		When("the /mcp-scoped well-known path is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/.well-known/oauth-protected-resource/mcp", "", nil)
			})

			It("returns the metadata document, not a 401", func() {
				fetchesSuccessfully()
			})
		})
	})
})
