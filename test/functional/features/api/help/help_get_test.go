package help_test

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support/api"
)

// Not part of api/openapi.yaml, so tested here directly rather than via a
// generated entity client - see internal/help.
var _ = Describe("Fetching the MCP guide", func() {
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
		When("the MCP guide is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/help/mcp", "", nil)
			})

			It("returns the page, not a 401", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
				body, readErr := io.ReadAll(resp.Body)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("<h1>Using mykeebs with an AI assistant</h1>"))
				Expect(string(body)).To(ContainSubstring("/mcp</code>"))
			})
		})
	})
})
