// Package cascadedelete implements the block/cascade on_delete
// policy for deleting an item that may still be referenced by one or more
// Builds. Both the REST handlers
// ([github.com/roguekeyboards/mykeebs-api/internal/handlers]) and the MCP tools
// ([github.com/roguekeyboards/mykeebs-api/internal/mcp]) call the DeleteX function
// for the entity being deleted directly and map its result/error to their
// own response shape, rather than each reimplementing the
// find-then-decide-then-delete flow.
package cascadedelete

import (
	"errors"
	"fmt"
	"strings"
)

// OnDelete controls how a DeleteX call behaves when the item is still
// referenced by one or more builds.
type OnDelete string

const (
	// OnDeleteBlock is the default: DeleteX fails with a *BlockedError if
	// the item is still referenced by any build.
	OnDeleteBlock OnDelete = "block"
	// OnDeleteCascade deletes the item and every build that references it,
	// in full.
	OnDeleteCascade OnDelete = "cascade"
)

// ParseOnDelete maps a raw on_delete value (a REST query param or MCP tool
// input field) to an OnDelete. Surrounding whitespace is trimmed first; an
// empty result defaults to OnDeleteBlock. ok is false if raw is non-empty
// and isn't block or cascade.
func ParseOnDelete(raw string) (onDelete OnDelete, ok bool) {
	trimmed := OnDelete(strings.TrimSpace(raw))
	switch trimmed {
	case "":
		return OnDeleteBlock, true
	case OnDeleteBlock, OnDeleteCascade:
		return trimmed, true
	default:
		return "", false
	}
}

// ErrBlocked is returned by a DeleteX call in block mode when the item is
// still referenced by at least one build. Callers use [errors.As] with
// *BlockedError to get the blocking build ids.
var ErrBlocked = errors.New("item is still referenced by at least one build")

// BlockedError carries the ids of the builds blocking a DeleteX call. It
// matches [errors.Is](err, ErrBlocked).
type BlockedError struct {
	BuildIDs []string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("still referenced by builds: %s", strings.Join(e.BuildIDs, ", "))
}

// Is reports whether target is [ErrBlocked].
func (e *BlockedError) Is(target error) bool {
	return target == ErrBlocked
}

// Result is a DeleteX call's success return value. DeletedBuildIDs is
// empty except in cascade mode.
type Result struct {
	DeletedBuildIDs []string
}
