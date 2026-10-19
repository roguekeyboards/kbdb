package mcp

import (
	"context"

	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type NewServerSuite struct {
	suite.Suite
}

func TestNewServerSuite(t *testing.T) {
	suite.Run(t, new(NewServerSuite))
}

func (s *NewServerSuite) TestAdvertisesToolsWithoutListChanged() {
	server := newServer("test")
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: "noop"}, func(_ context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
		return &sdkmcp.CallToolResult{}, nil, nil
	})
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(s.T().Context(), serverTransport, nil)
	s.Require().NoError(err)
	defer func() { _ = serverSession.Close() }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "client", Version: "test"}, nil)
	session, err := client.Connect(s.T().Context(), clientTransport, nil)
	s.Require().NoError(err)
	defer func() { _ = session.Close() }()

	tools := session.InitializeResult().Capabilities.Tools
	s.Require().NotNil(tools)
	s.False(tools.ListChanged)
}
