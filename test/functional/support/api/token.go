package api

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/roguekeyboards/oidc-testkit/pkg/oidctest"

	"github.com/roguekeyboards/kbdb/test/functional/support"
)

var (
	signerOnce sync.Once
	signer     *oidctest.Signer
	signerErr  error
)

func sharedSigner() (*oidctest.Signer, error) {
	signerOnce.Do(func() {
		path := support.OidcSigningKeyPath()
		if path == "" {
			signerErr = fmt.Errorf("KBDB_OIDC_SIGNING_KEY_PATH is not set - run scripts/func-setup.sh first")
			return
		}
		pemBytes, err := os.ReadFile(path) //nolint:gosec // path is a deploy-controlled env var
		if err != nil {
			signerErr = fmt.Errorf("reading signing key %s: %w", path, err)
			return
		}
		key, err := oidctest.LoadKey(pemBytes)
		if err != nil {
			signerErr = fmt.Errorf("parsing signing key %s: %w", path, err)
			return
		}
		signer = oidctest.NewSigner(key, support.OidcIssuer(), support.OidcAudience())
	})
	return signer, signerErr
}

// NewAuthIdentity mints a token for a fresh random subject and returns both,
// so a spec can seed and assert data owned by that subject.
func NewAuthIdentity(_ context.Context) (token, subject string, err error) {
	s, err := sharedSigner()
	if err != nil {
		return "", "", err
	}
	return s.Sign()
}
