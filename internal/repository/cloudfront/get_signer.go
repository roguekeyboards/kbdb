package cloudfront

import (
	"crypto"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"

	"github.com/rogueserenity/kbdb/internal/repository"
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
	signer := sign.NewURLSigner(keyID, key)
	signer.HashAlg = sign.HashSHA256

	return &GetSigner{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		signer:  signer,
		now:     time.Now,
	}
}

// Sign returns a signed URL for key and when it stops working.
func (g *GetSigner) Sign(key string) (string, time.Time, error) {
	expiresAt := g.now().Truncate(repository.ImageURLWindow).Add(repository.ImageURLLifetime)

	url, err := g.signer.Sign(g.baseURL+"/"+key, expiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing GET for %s: %w", key, err)
	}

	return url, expiresAt, nil
}
