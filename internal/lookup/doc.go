// Package lookup validates entity fields against approved lookup category
// values, independent of any transport (REST, MCP). It's the one place
// that knows which fields on which entities are open-vocabulary and which
// lookup category each maps to;
// [github.com/roguekeyboards/kbdb/internal/repository] stays
// data-access-only with no validation logic of its own.
package lookup
