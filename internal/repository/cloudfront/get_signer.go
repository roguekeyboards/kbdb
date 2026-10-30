package cloudfront

import (
	"crypto"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"

	"github.com/roguekeyboards/kbdb/internal/repository"
)

type urlSigner interface {
	Sign(url string, expires time.Time) (string, error)
}

// GetSigner signs image GET URLs on the images CloudFront distribution.
type GetSigner struct {
	baseURL string
	signer  urlSigner
	now     func() time.Time
}

// NewGetSigner returns a GetSigner for URLs under baseURL, signed with key,
// whose public half is registered with CloudFront as keyID.
func NewGetSigner(baseURL, keyID string, key crypto.Signer) *GetSigner {
	signer := sign.NewURLSigner(keyID, deterministicSigner{key})
	signer.HashAlg = sign.HashSHA256

	return &GetSigner{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		signer:  signer,
		now:     time.Now,
	}
}

// deterministicSigner signs without randomness, so an ECDSA key gives the
// same URL for the same key and expiry (RFC 6979) and browsers can cache it.
type deterministicSigner struct{ crypto.Signer }

func (d deterministicSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return d.Signer.Sign(nil, digest, opts)
}

// Sign returns a signed URL for key.
func (g *GetSigner) Sign(key string) (string, error) {
	expiresAt := g.now().Truncate(repository.ImageURLWindow).Add(repository.ImageURLLifetime)

	url, err := g.signer.Sign(g.baseURL+"/"+key, expiresAt)
	if err != nil {
		return "", fmt.Errorf("signing GET for %s: %w", key, err)
	}

	return url, nil
}
