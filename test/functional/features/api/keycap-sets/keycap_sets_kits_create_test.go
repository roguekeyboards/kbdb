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

var _ = Describe("Creating a keycap kit", func() {
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
			keycapSetID = "create-kit-set-" + uuid.NewString()
			Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			Context("given a valid request body", func() {
				When("creating a kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("creates the kit, embedded in the parent set", func(ctx SpecContext) {
						By("returning 201 Created")
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						By("returning a server-generated kit_id and null image")
						var got struct {
							KitID string    `json:"kit_id"`
							Name  string    `json:"name"`
							Image *struct{} `json:"image"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
						Expect(got.KitID).NotTo(BeEmpty())
						Expect(got.Name).To(Equal("Base"))
						Expect(got.Image).To(BeNil())

						By("showing the kit embedded in a follow-up GetKeycapSet")
						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(getResp.StatusCode).To(Equal(http.StatusOK))

						var set struct {
							Kits []struct {
								KitID string `json:"kit_id"`
								Name  string `json:"name"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(1))
						Expect(set.Kits[0].KitID).To(Equal(got.KitID))
						Expect(set.Kits[0].Name).To(Equal("Base"))
					})
				})
			})

			Context("given the set already has one kit", func() {
				BeforeEach(func(ctx SpecContext) {
					createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base"}`)
					Expect(err).NotTo(HaveOccurred())
					Expect(createResp.StatusCode).To(Equal(http.StatusCreated))
				})

				When("creating a second kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Extension"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("accumulates kits in the order they were added", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							Kits []struct {
								Name string `json:"name"`
							} `json:"kits"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.Kits).To(HaveLen(2))

						names := make([]string, len(set.Kits))
						for i, k := range set.Kits {
							names[i] = k.Name
						}
						Expect(names).To(Equal([]string{"Base", "Extension"}))
					})
				})
			})

			Context("given the set has a kit stored before kits had an order", func() {
				BeforeEach(func(ctx SpecContext) {
					// A kit id that sorts after any UUID, so ordering by id
					// would list the new kit first.
					Expect(db.SeedKeycapSetWithKit(ctx, ownerID, keycapSetID, "zzz-legacy-kit", "private")).To(Succeed())
				})

				When("creating a kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Extension"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("lists the new kit after the existing one", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

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
						Expect(set.Kits[0].KitID).To(Equal("zzz-legacy-kit"))
						Expect(set.Kits[1].Name).To(Equal("Extension"))
					})
				})
			})

			Context("given primary is true", func() {
				When("creating a kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base","primary":true}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("makes the new kit the set's primary_kit_id", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							KitID string `json:"kit_id"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							PrimaryKitID *string `json:"primary_kit_id"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.PrimaryKitID).NotTo(BeNil())
						Expect(*set.PrimaryKitID).To(Equal(created.KitID))
					})
				})
			})

			Context("given the set already has a primary kit and primary is true on a new kit", func() {
				var existingPrimaryKitID string

				BeforeEach(func(ctx SpecContext) {
					createResp, err := client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Base","primary":true}`)
					Expect(err).NotTo(HaveOccurred())
					Expect(createResp.StatusCode).To(Equal(http.StatusCreated))

					var created struct {
						KitID string `json:"kit_id"`
					}
					Expect(json.NewDecoder(createResp.Body).Decode(&created)).To(Succeed())
					existingPrimaryKitID = created.KitID
				})

				When("creating a second kit with primary true", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{"name":"Extension","primary":true}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("replaces the previous primary kit designation", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusCreated))

						var created struct {
							KitID string `json:"kit_id"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
						Expect(created.KitID).NotTo(Equal(existingPrimaryKitID))

						getResp, err := client.Get(ctx, ownerID, keycapSetID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var set struct {
							PrimaryKitID *string `json:"primary_kit_id"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&set)).To(Succeed())
						Expect(set.PrimaryKitID).NotTo(BeNil())
						Expect(*set.PrimaryKitID).To(Equal(created.KitID))
					})
				})
			})

			Context("given required fields are missing", func() {
				When("creating a kit", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.CreateKit(ctx, ownerID, keycapSetID, ownerToken, `{}`)
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

			When("creating a kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.CreateKit(ctx, ownerID, keycapSetID, token, `{"name":"Base"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the set exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("creating a kit", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.CreateKit(ctx, ownerID, keycapSetID, "", `{"name":"Base"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})
	})

	Context("given the parent keycap set does not exist", func() {
		When("creating a kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.CreateKit(ctx, ownerID, "no-such-keycap-set-"+uuid.NewString(), ownerToken, `{"name":"Base"}`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
