package keyboards_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Updating a keyboard", func() {
	var (
		resp       *http.Response
		client     *api.KeyboardsClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeyboardsClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a private keyboard owned by the caller", func() {
		var keyboardID string

		BeforeEach(func(ctx SpecContext) {
			keyboardID = "update-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("updating the keyboard", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken,
							`{"brand":"Keychron","name":"Q1 Pro","visibility":"private"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 200 with the updated keyboard, persisted", func(ctx SpecContext) {
						By("returning 200 OK")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						By("returning the keyboard's id and updated name")
						var got struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.ID).To(Equal(keyboardID))
						Expect(got.Name).To(Equal("Q1 Pro"))

						By("actually persisting the new name, not a no-op")
						getResp, err := client.Get(ctx, ownerID, keyboardID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))

						var reGot struct {
							Name string `json:"name"`
							keyboardWithParts
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&reGot)).To(Succeed())
						Expect(reGot.Name).To(Equal("Q1 Pro"))

						By("leaving the keyboard's plate and PCB as they were")
						Expect(reGot.Plates).To(ConsistOf(HaveField("ID", db.SeededPlateID)))
						Expect(reGot.PCBs).To(ConsistOf(HaveField("ID", db.SeededPCBID)))
					})
				})
			})

			Context("given a request body changing visibility to public", func() {
				When("updating the keyboard", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken,
							`{"brand":"Keychron","name":"Q1","visibility":"public"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the keyboard visible to an anonymous caller", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, keyboardID, "")
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("updating the keyboard", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken,
							`{"brand":"Keychron","name":"Q1","visibility":"private","size":"NotApproved"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given size is approved but not valid for the given layout", func() {
				When("updating the keyboard", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken,
							`{"brand":"Keychron","name":"Q1","visibility":"private","size":"`+approvedOtherSize+`","layout":"`+approvedLayout+`"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body naming only layout", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

						var got struct {
							InvalidParams []struct {
								Name string `json:"name"`
							} `json:"invalid_params"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						names := make([]string, len(got.InvalidParams))
						for i, p := range got.InvalidParams {
							names[i] = p.Name
						}
						Expect(names).To(ConsistOf("layout"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("updating the keyboard", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken, `{"brand":"Keychron"}`)
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

			When("updating the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keyboardID, token,
						`{"brand":"Keychron","name":"Q1","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("updating the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keyboardID, "",
						`{"brand":"Keychron","name":"Q1","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given an existing keyboard has an optional field set", func() {
		var keyboardID string

		BeforeEach(func(ctx SpecContext) {
			createResp, err := client.Create(ctx, ownerID, ownerToken,
				`{"brand":"Keychron","name":"Q1","visibility":"private","notes":"stock lubed"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			keyboardID = created.ID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		})

		Context("given a request body omitting that field", func() {
			When("updating the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keyboardID, ownerToken,
						`{"brand":"Keychron","name":"Q1","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("clears the omitted field, since PUT replaces rather than merges", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, keyboardID, ownerToken)
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

	Context("given the keyboard does not exist", func() {
		Context("given the caller is the owner", func() {
			When("updating the keyboard", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, "no-such-keyboard-"+uuid.NewString(), ownerToken,
						`{"brand":"Keychron","name":"Q1","visibility":"private"}`)
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
