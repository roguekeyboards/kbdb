// Package handlers holds the REST route handler functions, one file per
// entity. Each handler takes its repository interface from
// [github.com/roguekeyboards/mykeebs-api/internal/repository], maps to and from the
// spec-shaped types via [github.com/roguekeyboards/mykeebs-api/internal/repoapi],
// and reports failures as RFC 9457 responses via
// [github.com/roguekeyboards/mykeebs-api/internal/problem]. Router wiring lives in
// [github.com/roguekeyboards/mykeebs-api/internal/router], not here.
package handlers
