package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck,revive // Ginkgo's dot-import convention, matching every spec file
	. "github.com/onsi/gomega"    //nolint:staticcheck,revive // ditto

	"github.com/roguekeyboards/kbdb/test/functional/support"
)

// MCPClient speaks MCP over the streamable-HTTP transport, unlike Client's
// plain REST requests.
type MCPClient struct {
	client   *mcp.Client
	endpoint string
	rt       http.RoundTripper
}

// NewMCPClient returns a client targeting endpoint (e.g. "http://host/mcp").
// authToken, if non-empty, is sent as a Bearer token on every request.
func NewMCPClient(endpoint, authToken string) *MCPClient {
	return &MCPClient{
		client:   mcp.NewClient(&mcp.Implementation{Name: "kbdb-functional-test", Version: "1.0"}, nil),
		endpoint: endpoint,
		rt:       bearerTokenTransport{token: authToken},
	}
}

// CallTool opens a session and performs a tools/call request for the named
// tool. A verification failure (missing or invalid bearer token) surfaces
// as a plain error here, not a decoded result - the 2026-07-28 spec
// requires auth failures to be a real HTTP 401, not an MCP-shaped
// protocol-level error (see internal/mcp.requireBearerToken).
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: &http.Client{Transport: c.rt},
	}

	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting: %w", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", name, err)
	}
	return result, nil
}

// NewAuthenticatedMCPClient mints a fresh identity and returns an MCPClient
// authenticated as it, plus its subject - the setup every MCP spec needs for
// its primary caller.
func NewAuthenticatedMCPClient(ctx context.Context) (client *MCPClient, ownerID string) {
	GinkgoHelper()
	token, ownerID, err := NewAuthIdentity(ctx)
	Expect(err).NotTo(HaveOccurred())
	return NewMCPClient(support.BaseURL()+"/mcp", token), ownerID
}

// NewOtherUserID mints a fresh identity and returns just its subject, for
// specs that only need a second, unrelated owner to attribute fixture data
// to - not to act as that user.
func NewOtherUserID(ctx context.Context) string {
	GinkgoHelper()
	_, otherID, err := NewAuthIdentity(ctx)
	Expect(err).NotTo(HaveOccurred())
	return otherID
}

// bearerTokenTransport adds an Authorization header to every request, or
// none if token is empty.
type bearerTokenTransport struct {
	token string
}

func (t bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return http.DefaultTransport.RoundTrip(req)
}
