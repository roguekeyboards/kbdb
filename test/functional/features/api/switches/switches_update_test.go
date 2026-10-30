package switches_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Updating a switch", func() {
	var (
		resp       *http.Response
		client     *api.SwitchesClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewSwitchesClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a private switch owned by the caller", func() {
		var switchID string

		BeforeEach(func(ctx SpecContext) {
			switchID = "update-switch-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("updating the switch", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, switchID, ownerToken,
							`{"brand":"Gateron","name":"Yellow2","type":"Linear","visibility":"private"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 200 with the updated switch, persisted", func(ctx SpecContext) {
						By("returning 200 OK")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						By("returning the switch's id and updated name")
						var got struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.ID).To(Equal(switchID))
						Expect(got.Name).To(Equal("Yellow2"))

						By("actually persisting the new name, not a no-op")
						getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))

						var reGot struct {
							Name string `json:"name"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&reGot)).To(Succeed())
						Expect(reGot.Name).To(Equal("Yellow2"))
					})
				})
			})

			Context("given a request body changing visibility to public", func() {
				When("updating the switch", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, switchID, ownerToken,
							`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"public"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the switch visible to an anonymous caller", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, switchID, "")
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("updating the switch", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, switchID, ownerToken,
							`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private","material":{"stem":"NotApproved"}}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("updating the switch", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, switchID, ownerToken, `{"brand":"Gateron"}`)
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

			When("updating the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, switchID, token,
						`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("updating the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, switchID, "",
						`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given an existing switch has an optional field set", func() {
		var switchID string

		BeforeEach(func(ctx SpecContext) {
			createResp, err := client.Create(ctx, ownerID, ownerToken,
				`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private","notes":"smooth"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			switchID = created.ID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})

		Context("given a request body omitting that field", func() {
			When("updating the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, switchID, ownerToken,
						`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("clears the omitted field, since PUT replaces rather than merges", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
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

	Context("given the switch does not exist", func() {
		Context("given the caller is the owner", func() {
			When("updating the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, "no-such-switch-"+uuid.NewString(), ownerToken,
						`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not idempotent like delete", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})

	Context("given the switch was just deleted", func() {
		var switchID string

		BeforeEach(func(ctx SpecContext) {
			switchID = "update-after-delete-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())

			delResp, err := client.Delete(ctx, ownerID, switchID, ownerToken)
			Expect(err).NotTo(HaveOccurred())
			Expect(delResp.StatusCode).To(Equal(http.StatusNoContent))
		})

		Context("given the caller is the owner", func() {
			When("updating the switch", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, switchID, ownerToken,
						`{"brand":"Gateron","name":"Yellow","type":"Linear","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404 and does not resurrect the switch", func(ctx SpecContext) {
					By("returning 404 with a problem+json body")
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

					By("leaving the switch gone - the update did not recreate it")
					getResp, err := client.Get(ctx, ownerID, switchID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					Expect(getResp.StatusCode).To(Equal(http.StatusNotFound))
				})
			})
		})
	})
})
