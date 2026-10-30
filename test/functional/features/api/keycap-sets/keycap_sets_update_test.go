package keycapsets_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Updating a keycap set", func() {
	var (
		resp       *http.Response
		client     *api.KeycapSetsClient
		ownerID    string
		ownerToken string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeycapSetsClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given a private keycap set owned by the caller", func() {
		var keycapSetID string

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "update-keycap-set-" + uuid.NewString()
			Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("updating the keycap set", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken,
							`{"brand":"GMK","name":"Laser V2","visibility":"private"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 200 with the updated keycap set, persisted", func(ctx SpecContext) {
						By("returning 200 OK")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						By("returning the keycap set's id and updated name")
						var got struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.ID).To(Equal(keycapSetID))
						Expect(got.Name).To(Equal("Laser V2"))

						By("actually persisting the new name, not a no-op")
						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))

						var reGot struct {
							Name string `json:"name"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&reGot)).To(Succeed())
						Expect(reGot.Name).To(Equal("Laser V2"))
					})
				})
			})

			Context("given a request body changing visibility to public", func() {
				When("updating the keycap set", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken,
							`{"brand":"GMK","name":"Laser","visibility":"public"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the keycap set visible to an anonymous caller", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, "")
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("updating the keycap set", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken,
							`{"brand":"GMK","name":"Laser","visibility":"private","profile":"NotApproved"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("updating the keycap set", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken, `{"brand":"GMK"}`)
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

			When("updating the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keycapSetID, token,
						`{"brand":"GMK","name":"Laser","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("updating the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keycapSetID, "",
						`{"brand":"GMK","name":"Laser","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given an existing keycap set has an optional field set", func() {
		var keycapSetID string

		BeforeEach(func(ctx SpecContext) {
			createResp, err := client.Create(ctx, ownerID, ownerToken,
				`{"brand":"GMK","name":"Laser","visibility":"private","notes":"group buy"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			keycapSetID = created.ID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given a request body omitting that field", func() {
			When("updating the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken,
						`{"brand":"GMK","name":"Laser","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("clears the omitted field, since PUT replaces rather than merges", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
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

	Context("given an existing keycap set has a kit", func() {
		var keycapSetID string

		BeforeEach(func(ctx SpecContext) {
			createResp, err := client.Create(ctx, ownerID, ownerToken,
				`{"brand":"GMK","name":"Laser","visibility":"private"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			keycapSetID = created.ID

			kitResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(kitResp.StatusCode).To(Equal(http.StatusCreated))
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given an update to one of the set's own fields", func() {
			When("updating the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, keycapSetID, ownerToken,
						`{"brand":"GMK","name":"Laser V2","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("preserves the kit rather than wiping it, since Update must not clobber Kits", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
					Expect(err).NotTo(HaveOccurred())

					var got struct {
						Kits []struct {
							Name string `json:"name"`
						} `json:"kits"`
					}
					Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
					Expect(got.Kits).To(HaveLen(1))
					Expect(got.Kits[0].Name).To(Equal("Base"))
				})
			})
		})
	})

	Context("given the keycap set does not exist", func() {
		Context("given the caller is the owner", func() {
			When("updating the keycap set", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), ownerToken,
						`{"brand":"GMK","name":"Laser","visibility":"private"}`)
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
