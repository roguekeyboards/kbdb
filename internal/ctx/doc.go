// Package ctx holds request-scoped identity values carried through
// context.Context, as plain retrievable types — independent of logging (see
// [github.com/roguekeyboards/kbdb/internal/log]) or any particular transport
// (REST, MCP). Business logic that needs to know the current caller's user
// ID depends on this package directly, not on internal/log's
// request-logging plumbing.
package ctx
