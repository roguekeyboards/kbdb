// Package repoapi maps between
// [github.com/roguekeyboards/kbdb/internal/repository]'s DB-shaped types and
// [github.com/roguekeyboards/kbdb/internal/handlers/api]'s generated,
// spec-shaped types. Neither of those packages imports the other; this
// package imports both so the mapping is defined once per entity instead of
// inline in each handler.
package repoapi
