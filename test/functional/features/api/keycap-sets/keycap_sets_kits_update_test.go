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

var _ = Describe("Updating a keycap kit", func() {
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

	Context("given a private keycap set with an existing kit", func() {
		var (
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "update-kit-set-" + uuid.NewString()
			Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())

			createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

			var created struct {
				KitID string `json:"kit_id"`
			}
			Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
			kitID = created.KitID
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("updating the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{"name":"Base V2"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("updates the kit, embedded in the parent set", func(ctx SpecContext) {
						By("returning 200 OK with the updated name")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						var got struct {
							KitID string `json:"kit_id"`
							Name  string `json:"name"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.KitID).To(Equal(kitID))
						Expect(got.Name).To(Equal("Base V2"))

						By("showing the updated kit embedded in a follow-up GetKeycapSet")
						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							Kits []struct {
								KitID string `json:"kit_id"`
								Name  string `json:"name"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(1))
						Expect(set.Kits[0].KitID).To(Equal(kitID))
						Expect(set.Kits[0].Name).To(Equal("Base V2"))
					})
				})
			})

			Context("given the set has a second, sibling kit", func() {
				var siblingKitID string

				BeforeEach(func(ctx SpecContext) {
					createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Extension"}`)
					Expect(err).NotTo(HaveOccurred())
					Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

					var created struct {
						KitID string `json:"kit_id"`
					}
					Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
					siblingKitID = created.KitID
				})

				When("updating the first kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{"name":"Base V2"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("leaves the sibling kit unchanged, since the read-modify-write must not clobber other kits", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							Kits []struct {
								KitID string `json:"kit_id"`
								Name  string `json:"name"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(2))

						byID := map[string]string{}
						for _, k := range set.Kits {
							byID[k.KitID] = k.Name
						}
						Expect(byID[kitID]).To(Equal("Base V2"))
						Expect(byID[siblingKitID]).To(Equal("Extension"))
					})
				})
			})

			Context("given primary is true", func() {
				When("updating the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{"name":"Base V2","primary":true}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the kit the set's primary_kit_id", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							PrimaryKitID *string `json:"primary_kit_id"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.PrimaryKitID).NotTo(BeNil())
						Expect(*set.PrimaryKitID).To(Equal(kitID))
					})
				})
			})

			Context("given the kit is the set's current primary kit and primary is false", func() {
				BeforeEach(func(ctx SpecContext) {
					setPrimaryResp, err := client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{"name":"Base","primary":true}`)
					Expect(err).NotTo(HaveOccurred())
					Expect(setPrimaryResp.StatusCode).To(Equal(http.StatusOK))
				})

				When("updating the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{"name":"Base V2","primary":false}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("clears the set's primary_kit_id", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							PrimaryKitID *string `json:"primary_kit_id"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.PrimaryKitID).To(BeNil())
					})
				})
			})

			Context("given required fields are missing", func() {
				When("updating the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, ownerToken, `{}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 400 with a problem+json body", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the kitId does not exist within the set", func() {
				When("updating the kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, "no-such-kit-"+uuid.NewString(), ownerToken, `{"name":"Base V2"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 404", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
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

			When("updating the kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, token, `{"name":"Base V2"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the set exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("updating the kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.UpdateKit(ctx, ownerID, keycapSetID, kitID, "", `{"name":"Base V2"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the parent keycap set does not exist", func() {
		When("updating a kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.UpdateKit(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), "some-kit-id", ownerToken, `{"name":"Base V2"}`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
