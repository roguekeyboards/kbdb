// Package schema holds MCP tool input/output types. Kept separate from
// [github.com/roguekeyboards/kbdb/internal/handlers/api]'s generated REST
// types since mcp.AddTool infers tool schemas from jsonschema struct tags,
// which the generated types don't carry.
package schema
