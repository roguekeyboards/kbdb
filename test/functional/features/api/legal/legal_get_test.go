package legal_test

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support/api"
)

// Not part of api/openapi.yaml, so tested here directly rather than via a
// generated entity client - see internal/legal.
var _ = Describe("Fetching the privacy policy and terms of service", func() {
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
		When("the privacy policy is fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/privacy", "", nil)
			})

			It("returns the page, not a 401", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
				body, readErr := io.ReadAll(resp.Body)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("<h1>Privacy Policy</h1>"))
			})
		})

		When("the terms of service are fetched", func() {
			BeforeEach(func(ctx SpecContext) {
				resp, err = client.Do(ctx, http.MethodGet, "/terms", "", nil)
			})

			It("returns the page, not a 401", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
				body, readErr := io.ReadAll(resp.Body)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("<h1>Terms of Service</h1>"))
			})
		})
	})
})
