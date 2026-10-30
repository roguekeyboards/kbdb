package builds_test

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Listing builds", func() {
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

	type listItem struct {
		ID       string `json:"id"`
		Keyboard *struct {
			ID    string `json:"id"`
			Brand string `json:"brand"`
			Name  string `json:"name"`
		} `json:"keyboard"`
		TotalCost *float64 `json:"total_cost"`
		Currency  *string  `json:"currency"`
		Stabs     *struct {
			Name     *string  `json:"name"`
			Price    *float64 `json:"price"`
			Currency *string  `json:"currency"`
		} `json:"stabs"`
	}

	decodeItems := func(r *http.Response) []listItem {
		var page struct {
			Items []listItem `json:"items"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&page)).To(Succeed())
		return page.Items
	}

	decodePage := func(r *http.Response) ([]string, string) {
		var page struct {
			Items      []listItem `json:"items"`
			NextCursor string     `json:"next_cursor"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&page)).To(Succeed())
		ids := make([]string, len(page.Items))
		for i, item := range page.Items {
			ids[i] = item.ID
		}
		return ids, page.NextCursor
	}

	itemIDs := func(items []listItem) []string {
		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.ID
		}
		return ids
	}

	Context("given the owner has no builds", func() {
		When("listing builds", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.List(ctx, ownerID, ownerToken, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an empty page", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeItems(resp)).To(BeEmpty())
			})
		})
	})

	Context("given the owner has builds at every visibility tier", func() {
		var publicID, authenticatedID, privateID string

		BeforeEach(func(ctx SpecContext) {
			publicID = "public-build-" + uuid.NewString()
			authenticatedID = "authenticated-build-" + uuid.NewString()
			privateID = "private-build-" + uuid.NewString()

			Expect(db.SeedBuild(ctx, ownerID, publicID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, authenticatedID, keyboardID, "authenticated")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, privateID, keyboardID, "private")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, publicID, keyboardID)).To(Succeed())
			Expect(db.DeleteBuild(ctx, ownerID, authenticatedID, keyboardID)).To(Succeed())
			Expect(db.DeleteBuild(ctx, ownerID, privateID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns builds at every visibility tier, each with the denormalized keyboard brand/name", func() {
					By("returning 200 OK")
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					items := decodeItems(resp)
					By("including all three seeded builds")
					Expect(itemIDs(items)).To(ContainElements(publicID, authenticatedID, privateID))

					By("denormalizing the referenced keyboard's brand/name onto every item")
					for _, item := range items {
						Expect(item.Keyboard).NotTo(BeNil())
						Expect(item.Keyboard.ID).To(Equal(keyboardID))
						Expect(item.Keyboard.Brand).To(Equal("Keychron"))
						Expect(item.Keyboard.Name).To(Equal("Q1"))
					}
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, "", -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns only the public build", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					ids := itemIDs(decodeItems(resp))
					Expect(ids).To(ContainElement(publicID))
					Expect(ids).NotTo(ContainElement(authenticatedID))
					Expect(ids).NotTo(ContainElement(privateID))
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

			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the public and authenticated builds, but not the private one", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					ids := itemIDs(decodeItems(resp))
					Expect(ids).To(ContainElements(publicID, authenticatedID))
					Expect(ids).NotTo(ContainElement(privateID))
				})
			})
		})
	})

	Context("given the owner has a build with priced components", func() {
		var buildID string

		BeforeEach(func(ctx SpecContext) {
			buildID = "priced-build-" + uuid.NewString()
			Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes total_cost derived from the keyboard's price and the stabs' price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					items := decodeItems(resp)
					idx := slices.IndexFunc(items, func(i listItem) bool { return i.ID == buildID })
					Expect(idx).To(BeNumerically(">=", 0))
					Expect(items[idx].TotalCost).NotTo(BeNil())
					Expect(*items[idx].TotalCost).To(Equal(329.99 + 12.5))
					Expect(items[idx].Currency).To(HaveValue(Equal("USD")))

					By("returning the build in full, including its stabs")
					Expect(items[idx].Stabs).NotTo(BeNil())
					Expect(items[idx].Stabs.Name).To(HaveValue(Equal("Durock v3")))
					Expect(items[idx].Stabs.Price).To(HaveValue(Equal(12.5)))
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

			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits total_cost and the stabs price even though the build itself is visible", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					items := decodeItems(resp)
					idx := slices.IndexFunc(items, func(i listItem) bool { return i.ID == buildID })
					Expect(idx).To(BeNumerically(">=", 0))
					Expect(items[idx].TotalCost).To(BeNil())
					Expect(items[idx].Currency).To(BeNil())
					Expect(items[idx].Stabs).NotTo(BeNil(), "stripping the price must keep the rest of stabs")
					Expect(items[idx].Stabs.Price).To(BeNil())
					Expect(items[idx].Stabs.Currency).To(BeNil())
					Expect(items[idx].Stabs.Name).To(HaveValue(Equal("Durock v3")))
				})
			})
		})
	})

	Context("given the owner has a priced build and show_price_to_me is false", func() {
		var buildID, profileUsername string

		BeforeEach(func(ctx SpecContext) {
			buildID = "priced-build-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "public")).To(Succeed())
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
			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, ownerToken, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("omits total_cost and the stabs price", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					items := decodeItems(resp)
					idx := slices.IndexFunc(items, func(i listItem) bool { return i.ID == buildID })
					Expect(idx).To(BeNumerically(">=", 0))
					Expect(items[idx].TotalCost).To(BeNil())
					Expect(items[idx].Currency).To(BeNil())
					Expect(items[idx].Stabs).NotTo(BeNil(), "stripping the price must keep the rest of stabs")
					Expect(items[idx].Stabs.Price).To(BeNil())
					Expect(items[idx].Stabs.Currency).To(BeNil())
					Expect(items[idx].Stabs.Name).To(HaveValue(Equal("Durock v3")))
				})
			})
		})
	})

	Context("given the owner has a priced build and show_price_to_others is true", func() {
		var buildID, profileUsername string

		BeforeEach(func(ctx SpecContext) {
			buildID = "priced-build-" + uuid.NewString()
			profileUsername = "u" + uuid.NewString()[:8]
			Expect(db.SeedBuildWithStabs(ctx, ownerID, buildID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: profileUsername,
				Preferences: map[string]any{
					"currency": "EUR", "show_price_to_me": true, "show_price_to_others": true,
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

			When("listing builds", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.List(ctx, ownerID, token, -1)
					Expect(err).NotTo(HaveOccurred())
				})

				It("includes total_cost", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					items := decodeItems(resp)
					idx := slices.IndexFunc(items, func(i listItem) bool { return i.ID == buildID })
					Expect(idx).To(BeNumerically(">=", 0))
					Expect(items[idx].TotalCost).NotTo(BeNil())
					Expect(*items[idx].TotalCost).To(Equal(329.99 + 12.5))
					Expect(items[idx].Currency).To(HaveValue(Equal("EUR")))
				})
			})
		})
	})

	Context("given the owner has more builds than fit under a small limit, each referencing the keyboard", func() {
		var buildIDs []string

		BeforeEach(func(ctx SpecContext) {
			buildIDs = nil
			for range 5 {
				id := "paged-build-" + uuid.NewString()
				Expect(db.SeedBuild(ctx, ownerID, id, keyboardID, "public")).To(Succeed())
				buildIDs = append(buildIDs, id)
			}
		})

		AfterEach(func(ctx SpecContext) {
			for _, id := range buildIDs {
				Expect(db.DeleteBuild(ctx, ownerID, id, keyboardID)).To(Succeed())
			}
		})

		When("listing builds with a limit smaller than the build count", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.List(ctx, ownerID, ownerToken, 3)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a full page of builds, not markers filtered down to a partial page", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeItems(resp)).To(HaveLen(3))
			})
		})
	})

	Context("given the owner has builds referencing two different keyboards", func() {
		var otherKeyboardID, matchingBuildID, otherBuildID string

		BeforeEach(func(ctx SpecContext) {
			otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
			matchingBuildID = "matching-build-" + uuid.NewString()
			otherBuildID = "other-build-" + uuid.NewString()

			Expect(db.SeedKeyboard(ctx, ownerID, otherKeyboardID, "private")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, matchingBuildID, keyboardID, "public")).To(Succeed())
			Expect(db.SeedBuild(ctx, ownerID, otherBuildID, otherKeyboardID, "public")).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteBuild(ctx, ownerID, matchingBuildID, keyboardID)).To(Succeed())
			Expect(db.DeleteBuild(ctx, ownerID, otherBuildID, otherKeyboardID)).To(Succeed())
			Expect(db.DeleteKeyboard(ctx, ownerID, otherKeyboardID)).To(Succeed())
		})

		When("listing builds filtered to one of the keyboards", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.ListFiltered(ctx, ownerID, ownerToken, keyboardID, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns only the build referencing that keyboard", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				ids := itemIDs(decodeItems(resp))
				Expect(ids).To(ContainElement(matchingBuildID))
				Expect(ids).NotTo(ContainElement(otherBuildID))
			})
		})
	})

	Context("given the owner has no builds referencing a given keyboard", func() {
		When("listing builds filtered to that keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.ListFiltered(ctx, ownerID, ownerToken, keyboardID, -1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an empty page", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(decodeItems(resp)).To(BeEmpty())
			})
		})
	})

	Context("given the owner's builds for a keyboard are spread among builds for another keyboard", func() {
		var otherKeyboardID string
		matching := []string{"build-2", "build-5", "build-7"}
		others := []string{"build-1", "build-3", "build-4", "build-6"}

		BeforeEach(func(ctx SpecContext) {
			otherKeyboardID = "build-fixture-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, otherKeyboardID, "private")).To(Succeed())
			for _, id := range matching {
				Expect(db.SeedBuild(ctx, ownerID, id, keyboardID, "public")).To(Succeed())
			}
			for _, id := range others {
				Expect(db.SeedBuild(ctx, ownerID, id, otherKeyboardID, "public")).To(Succeed())
			}
		})

		AfterEach(func(ctx SpecContext) {
			for _, id := range matching {
				Expect(db.DeleteBuild(ctx, ownerID, id, keyboardID)).To(Succeed())
			}
			for _, id := range others {
				Expect(db.DeleteBuild(ctx, ownerID, id, otherKeyboardID)).To(Succeed())
			}
			Expect(db.DeleteKeyboard(ctx, ownerID, otherKeyboardID)).To(Succeed())
		})

		When("paging through builds filtered to that keyboard two at a time", func() {
			var firstIDs, secondIDs []string
			var firstCursor, secondCursor string

			BeforeEach(func(ctx SpecContext) {
				first, err := client.ListPage(ctx, ownerID, ownerToken, keyboardID, 2, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(first.StatusCode).To(Equal(http.StatusOK))
				firstIDs, firstCursor = decodePage(first)

				second, err := client.ListPage(ctx, ownerID, ownerToken, keyboardID, 2, firstCursor)
				Expect(err).NotTo(HaveOccurred())
				Expect(second.StatusCode).To(Equal(http.StatusOK))
				secondIDs, secondCursor = decodePage(second)
			})

			It("fills every page but the last and returns every matching build", func() {
				Expect(firstIDs).To(Equal([]string{"build-2", "build-5"}))
				Expect(firstCursor).NotTo(BeEmpty())
				Expect(secondIDs).To(Equal([]string{"build-7"}))
				Expect(secondCursor).To(BeEmpty())
			})
		})

		Context("given a cursor issued while filtering to that keyboard", func() {
			var cursor string

			BeforeEach(func(ctx SpecContext) {
				first, err := client.ListPage(ctx, ownerID, ownerToken, keyboardID, 1, "")
				Expect(err).NotTo(HaveOccurred())
				_, cursor = decodePage(first)
				Expect(cursor).NotTo(BeEmpty())
			})

			When("listing builds filtered to the other keyboard with that cursor", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.ListPage(ctx, ownerID, ownerToken, otherKeyboardID, 1, cursor)
					Expect(err).NotTo(HaveOccurred())
				})

				It("rejects the cursor", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				})
			})
		})
	})

	Context("given the owner's public builds sort after private ones", func() {
		private := []string{"build-1", "build-2", "build-3"}
		public := []string{"build-4", "build-5"}

		BeforeEach(func(ctx SpecContext) {
			for _, id := range private {
				Expect(db.SeedBuild(ctx, ownerID, id, keyboardID, "private")).To(Succeed())
			}
			for _, id := range public {
				Expect(db.SeedBuild(ctx, ownerID, id, keyboardID, "public")).To(Succeed())
			}
		})

		AfterEach(func(ctx SpecContext) {
			for _, id := range append(private, public...) {
				Expect(db.DeleteBuild(ctx, ownerID, id, keyboardID)).To(Succeed())
			}
		})

		When("an anonymous caller lists two builds", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.ListPage(ctx, ownerID, "", "", 2, "")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns both public builds in the first page", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				ids, _ := decodePage(resp)
				Expect(ids).To(Equal([]string{"build-4", "build-5"}))
			})
		})
	})

	DescribeTable("given an invalid limit",
		func(ctx SpecContext, limit int) {
			var err error
			resp, err = client.List(ctx, ownerID, ownerToken, limit)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
		},
		Entry("below the minimum", 0),
		Entry("above the maximum", 101),
	)
})
