package authorize_test

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/internal/consent"
	"github.com/rogueserenity/kbdb/test/functional/support/api"
)

// Not part of api/openapi.yaml, so tested here directly rather than via a
// generated entity client - see internal/consent.
var _ = Describe("Fetching the IdP consent page", func() {
	var (
		client *api.Client
		resp   *http.Response
		err    error
	)

	BeforeEach(func() {
		client = api.NewClient()
		resp = nil
		err = nil
	})

	Context("given no Authorization header", func() {
		When("the page is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/authorize", "", nil)
			})

			It("returns the page, not a 401", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
			})

			It("renders this stack's IdP consent public token into the page", func() {
				body, readErr := io.ReadAll(resp.Body)
				Expect(readErr).NotTo(HaveOccurred())
				// Matches scripts/func-setup.sh's IdpConsentPublicToken deploy
				// parameter for this stack.
				Expect(string(body)).To(ContainSubstring(`"public-token-test-local-kbdb"`))
			})
		})

		When("the page's IdP browser SDK is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, consent.IDPScriptPath, "", nil)
			})

			It("returns the script", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/javascript; charset=utf-8"))
				body, readErr := io.ReadAll(resp.Body)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(body)).To(HavePrefix("var stytch="))
			})
		})
	})

	Context("given a POST request", func() {
		When("the page is requested", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodPost, "/authorize", "", nil)
			})

			It("returns 405 Method Not Allowed", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
			})
		})
	})
})
