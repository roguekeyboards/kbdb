package repomcp

import (
	"github.com/roguekeyboards/kbdb/internal/lookup"
	"github.com/roguekeyboards/kbdb/internal/mcp/schema"
)

// Lookup maps lookup.Lookup to its MCP tool shape. It has no dependencies -
// lookups are static, deploy-time data.
type Lookup struct{}

// ToMCP maps a lookup.Lookup to its wire representation.
func (Lookup) ToMCP(l lookup.Lookup) schema.GetLookupOutput {
	return schema.GetLookupOutput{
		Category: string(l.Category),
		Values:   l.Values,
	}
}
