package moved_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
)

var _ = Describe("Fetching a page that moved to mykeebs.com", func() {
	noFollow := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	var (
		resp *http.Response
		err  error
	)

	BeforeEach(func() {
		resp = nil
		err = nil
	})

	fetch := func(ctx SpecContext, path string) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, support.BaseURL()+path, nil)
		Expect(reqErr).NotTo(HaveOccurred())
		resp, err = noFollow.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}

	Context("given no Authorization header", func() {
		When("the privacy policy is fetched", func() {
			BeforeEach(func(ctx SpecContext) { fetch(ctx, "/privacy") })

			It("redirects permanently to mykeebs.com", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusMovedPermanently))
				Expect(resp.Header.Get("Location")).To(Equal("https://mykeebs.com/privacy"))
			})
		})

		When("the terms of service are fetched", func() {
			BeforeEach(func(ctx SpecContext) { fetch(ctx, "/terms") })

			It("redirects permanently to mykeebs.com", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusMovedPermanently))
				Expect(resp.Header.Get("Location")).To(Equal("https://mykeebs.com/terms"))
			})
		})

		When("the MCP guide is fetched", func() {
			BeforeEach(func(ctx SpecContext) { fetch(ctx, "/help/mcp") })

			It("redirects permanently to mykeebs.com", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusMovedPermanently))
				Expect(resp.Header.Get("Location")).To(Equal("https://mykeebs.com/help/mcp"))
			})
		})
	})
})
