// Package mcp wires the official modelcontextprotocol/go-sdk MCP server into
// the application, reusing the same [auth.Verifier] as REST rather than a
// second verification implementation.
// [github.com/roguekeyboards/kbdb/internal/auth] holds all
// protocol-agnostic verification logic shared by this package and
// [github.com/roguekeyboards/kbdb/internal/middleware] (the REST-side
// adapter); neither adapter depends on the other. Request-scoped identity
// ([github.com/roguekeyboards/kbdb/internal/ctx]) and logging
// ([github.com/roguekeyboards/kbdb/internal/log]) are likewise independent,
// transport-agnostic packages this one depends on directly, so MCP tool
// logs get the same user_id/request_id correlation fields as REST without
// depending on the middleware package at all.
package mcp
