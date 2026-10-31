package builds_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Updating a build", func() {
	var (
		resp       *http.Response
		client     *api.BuildsClient
		ownerID    string
		ownerToken string
		keyboardID string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewBuildsClient()
		keyboardID = "build-fixture-keyboard-" + uuid.NewString()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given a private build owned by the caller", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = "update-build-" + uuid.NewString()
			Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("updating the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, buildID, ownerToken,
							`{"keyboard":"`+keyboardID+`","visibility":"private","notes":"rebuilt"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 200 with the updated build, persisted", func(ctx SpecContext) {
						By("returning 200 OK")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						By("returning the build's id and updated notes")
						var got struct {
							ID    string `json:"id"`
							Notes string `json:"notes"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.ID).To(Equal(buildID))
						Expect(got.Notes).To(Equal("rebuilt"))

						By("actually persisting the new notes, not a no-op")
						getResp, err := client.Get(ctx, ownerID, buildID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))

						var reGot struct {
							Notes string `json:"notes"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&reGot)).To(Succeed())
						Expect(reGot.Notes).To(Equal("rebuilt"))
					})
				})
			})

			Context("given a request body changing visibility to public", func() {
				When("updating the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, buildID, ownerToken,
							`{"keyboard":"`+keyboardID+`","visibility":"public"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the build visible to an anonymous caller", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, buildID, "")
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("updating the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, buildID, ownerToken,
							`{"keyboard":"`+keyboardID+`","visibility":"private","stabs":{"name":"NotApproved"}}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("updating the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, buildID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the build references a keyboard that doesn't exist", func() {
				When("updating the build", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, buildID, ownerToken,
							`{"keyboard":"does-not-exist-`+uuid.NewString()+`","visibility":"private"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})
		})

		Context("given the caller is a different authenticated user", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("updating the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, buildID, token,
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("updating the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, buildID, "",
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given an existing build has an optional field set", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			createResp, err := client.Create(ctx, ownerID, ownerToken,
				`{"keyboard":"`+keyboardID+`","visibility":"private","notes":"group buy"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			buildID = created.ID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given a request body omitting that field", func() {
			When("updating the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, buildID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("clears the omitted field, since PUT replaces rather than merges", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, buildID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var got struct {
						Notes *string `json:"notes"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
					Expect(got.Notes).To(BeNil())
				})
			})
		})
	})

	Context("given the build does not exist", func() {
		Context("given the caller is the owner", func() {
			When("updating the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, "no-such-build-"+uuid.NewString(), ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not idempotent like delete", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})
})
