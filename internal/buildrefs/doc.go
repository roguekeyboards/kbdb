// Package buildrefs validates that a Build's cross-entity references
// (Keyboard, Switches, KeycapKits) name real resources owned by the
// caller, and that its Plate and PCB are ones its keyboard has. Separate
// from [github.com/roguekeyboards/mykeebs-api/internal/lookup] since it checks
// live repository data, not the static lookup category system.
package buildrefs
