// Package authz handles authorization: deciding what an already-identified
// caller (see [github.com/roguekeyboards/mykeebs-api/internal/auth]) may read or
// write. It does not verify tokens or establish identity itself.
package authz
