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

var _ = Describe("Getting a build", func() {
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

	seedBuild := func(ctx SpecContext, visibility string) string {
		id := visibility + "-build-" + uuid.NewString()
		Expect(db.SeedBuild(ctx, ownerID, id, keyboardID, visibility)).To(Succeed())
		return id
	}

	Context("given a public build", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = seedBuild(ctx, "public")
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given the caller is anonymous", func() {
			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the build without total_cost", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got struct {
						ID        string   `json:"id"`
						TotalCost *float64 `json:"total_cost"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.ID).To(Equal(buildID))
					Expect(got.TotalCost).To(BeNil())
				})
			})
		})

		Context("given the caller is the owner", func() {
			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the build with total_cost derived from the referenced keyboard's price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got struct {
						TotalCost *float64 `json:"total_cost"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.TotalCost).NotTo(BeNil())
					Expect(*got.TotalCost).To(Equal(329.99))
				})
			})
		})
	})

	Context("given a public build with priced stabs and the owner has show_price_to_others true", func() {
		var (
			buildID, profileUsername string
		)

		BeforeEach(func(ctx SpecContext) {
			buildID = "public-build-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "USD", "show_price_to_me": true, "show_price_to_others": true,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is not the owner", func() {
			var token string

			BeforeEach(func(ctx SpecContext) {
				var err error
				token, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the stabs price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got struct {
						Stabs *struct {
							Price *float64 `json:"price"`
						} `json:"stabs"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Stabs).NotTo(BeNil())
					Expect(got.Stabs.Price).NotTo(BeNil())
					Expect(*got.Stabs.Price).To(Equal(12.5))
				})
			})
		})
	})

	Context("given a private build with priced stabs and the owner has show_price_to_me false", func() {
		var (
			buildID, profileUsername string
		)

		BeforeEach(func(ctx SpecContext) {
			buildID = "private-build-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "USD", "show_price_to_me": false, "show_price_to_others": false,
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			Expect(db.DeleteProfile(ctx, ownerID, profileUsername)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("still returns the stabs price - single-item GET always shows the owner their own price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got struct {
						Stabs *struct {
							Price *float64 `json:"price"`
						} `json:"stabs"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Stabs).NotTo(BeNil())
					Expect(got.Stabs.Price).NotTo(BeNil())
					Expect(*got.Stabs.Price).To(Equal(12.5))
				})
			})
		})
	})

	Context("given a private build", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = seedBuild(ctx, "private")
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the build", func() {
					By("returning 200 OK")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					By("returning the build's id")
					var got struct {
						ID string `json:"id"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.ID).To(Equal(buildID))
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

			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, token)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing the item exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("getting the build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, buildID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})

	Context("given the build does not exist", func() {
		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, "no-such-build-"+uuid.NewString(), ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})

	Context("given a build referencing a keyboard that still exists", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = seedBuild(ctx, "private")
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("denormalizes the referenced keyboard's brand and name", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var got struct {
					Keyboard *struct {
						ID    string `json:"id"`
						Brand string `json:"brand"`
						Name  string `json:"name"`
					} `json:"keyboard"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Keyboard).NotTo(BeNil())
				Expect(got.Keyboard.ID).To(Equal(keyboardID))
				Expect(got.Keyboard.Brand).To(Equal("Keychron"))
				Expect(got.Keyboard.Name).To(Equal("Q1"))
			})
		})
	})

	Context("given a build referencing a switch that still exists", func() {
		var (
			buildID  string
			switchID string
		)

		BeforeEach(func(ctx SpecContext) {
			switchID = "build-fixture-switch-" + uuid.NewString()
			buildID = "private-build-" + uuid.NewString()
			Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
			Expect(db.SeedBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, switchID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, switchID)).To(Succeed())
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		})

		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("denormalizes the referenced switch's brand and name", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var got struct {
					Switches []struct {
						Switch *struct {
							ID    string `json:"id"`
							Brand string `json:"brand"`
							Name  string `json:"name"`
						} `json:"switch"`
						Count int `json:"count"`
					} `json:"switches"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Switches).To(HaveLen(1))
				Expect(got.Switches[0].Switch).NotTo(BeNil())
				Expect(got.Switches[0].Switch.ID).To(Equal(switchID))
				Expect(got.Switches[0].Switch.Brand).To(Equal("Gateron"))
				Expect(got.Switches[0].Switch.Name).To(Equal("Yellow"))
				Expect(got.Switches[0].Count).To(Equal(1))
			})
		})
	})

	Context("given a build referencing a keycap kit that still exists", func() {
		var (
			buildID     string
			keycapSetID string
			kitID       string
		)

		BeforeEach(func(ctx SpecContext) {
			keycapSetID = "build-fixture-keycap-set-" + uuid.NewString()
			kitID = "kit-" + uuid.NewString()
			buildID = "private-build-" + uuid.NewString()
			Expect(db.SeedKeycapSetWithKit(ctx, ownerID, keycapSetID, kitID, "private")).To(Succeed())
			Expect(db.SeedBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID, keyboardID, keycapSetID, kitID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID, keyboardID, keycapSetID, kitID)).To(Succeed())
			Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
		})

		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("groups the referenced kit under its denormalized keycap set", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var got struct {
					KeycapSets []struct {
						ID    string `json:"id"`
						Brand string `json:"brand"`
						Name  string `json:"name"`
						Kits  []struct {
							KitID string `json:"kit_id"`
							Name  string `json:"name"`
						} `json:"kits"`
					} `json:"keycap_sets"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.KeycapSets).To(HaveLen(1))
				Expect(got.KeycapSets[0].ID).To(Equal(keycapSetID))
				Expect(got.KeycapSets[0].Brand).To(Equal("GMK"))
				Expect(got.KeycapSets[0].Name).To(Equal("Laser"))
				Expect(got.KeycapSets[0].Kits).To(HaveLen(1))
				Expect(got.KeycapSets[0].Kits[0].KitID).To(Equal(kitID))
				Expect(got.KeycapSets[0].Kits[0].Name).To(Equal("Base"))
			})
		})
	})

	Context("given a build referencing a keyboard that has an image", func() {
		var (
			buildID   string
			imageKbID string
			imageID   string
		)

		BeforeEach(func(ctx SpecContext) {
			imageKbID = "build-fixture-keyboard-with-image-" + uuid.NewString()
			imageID = "img-" + uuid.NewString()
			buildID = "private-build-" + uuid.NewString()
			Expect(db.SeedKeyboardWithImage(ctx, ownerID, imageKbID, imageID, "private")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, buildID, imageKbID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, imageKbID)).To(Succeed())
			Expect(db.DeleteKeyboard(ctx, ownerID, imageKbID)).To(Succeed())
		})

		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("includes a presigned image_url for the keyboard's first image", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var got struct {
					Keyboard *struct {
						ImageURL *string `json:"image_url"`
					} `json:"keyboard"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Keyboard).NotTo(BeNil())
				Expect(got.Keyboard.ImageURL).NotTo(BeNil())
				Expect(*got.Keyboard.ImageURL).NotTo(BeEmpty())
			})
		})
	})

	Context("given a build referencing a switch that has an image", func() {
		var (
			buildID       string
			imageSwitchID string
		)

		BeforeEach(func(ctx SpecContext) {
			imageSwitchID = "build-fixture-switch-with-image-" + uuid.NewString()
			buildID = "private-build-" + uuid.NewString()
			Expect(db.SeedSwitchWithImage(ctx, ownerID, imageSwitchID, "private")).To(Succeed())
			Expect(db.SeedBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, imageSwitchID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, imageSwitchID)).To(Succeed())
			Expect(db.DeleteSwitch(ctx, ownerID, imageSwitchID)).To(Succeed())
		})

		When("getting the build", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("includes a presigned image_url for the switch", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var got struct {
					Switches []struct {
						Switch *struct {
							ImageURL *string `json:"image_url"`
						} `json:"switch"`
					} `json:"switches"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Switches).To(HaveLen(1))
				Expect(got.Switches[0].Switch).NotTo(BeNil())
				Expect(got.Switches[0].Switch.ImageURL).NotTo(BeNil())
				Expect(*got.Switches[0].Switch.ImageURL).NotTo(BeEmpty())
			})
		})
	})
})
