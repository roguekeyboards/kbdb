package profiles_test

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Updating a profile", func() {
	var (
		resp       *http.Response
		client     *api.ProfilesClient
		ownerID    string
		ownerToken string
		username   string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewProfilesClient()
		username = "u" + uuid.NewString()[:8]

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("given the caller has a profile", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:        username,
				Discoverable:    true,
				Bio:             "original bio",
				DiscordUsername: "alice_kb",
				Links: []map[string]string{
					{"name": "Twitch", "url": "https://twitch.tv/alice"},
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			// The rename cases leave the claim under a different username;
			// clean both up best-effort.
			_ = db.DeleteProfile(ctx, ownerID, username)
		})

		Context("given a PUT that changes the bio", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(`{
						"username": %q,
						"discoverable": true,
						"bio": "brand new bio",
						"links": [{"name": "Twitch", "url": "https://twitch.tv/alice"}]
					}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 with the changed bio", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Bio).NotTo(BeNil())
					Expect(*got.Bio).To(Equal("brand new bio"))
				})
			})
		})

		Context("given a PUT that omits links", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true, "bio": "original bio"}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 and clears the links", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Links).To(BeNil())
				})
			})
		})

		Context("given a PUT that changes the username", func() {
			var newUsername string

			BeforeEach(func() {
				newUsername = "u" + uuid.NewString()[:8]
			})

			AfterEach(func(ctx SpecContext) {
				_ = db.DeleteProfile(ctx, ownerID, newUsername)
			})

			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true}`, newUsername))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 and moves the username claim", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					byNew, err := client.Get(ctx, newUsername, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(byNew.StatusCode).To(Equal(http.StatusOK))

					byOld, err := client.Get(ctx, username, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(byOld.StatusCode).To(Equal(http.StatusNotFound))
				})
			})
		})

		Context("given a PUT with the same username", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true, "bio": "no rename"}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 with no self-conflict on the username claim", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				})
			})
		})

		Context("given a PUT that changes preferences", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(`{
						"username": %q,
						"discoverable": true,
						"preferences": {"currency": "GBP", "show_price_to_me": false, "show_price_to_others": true}
					}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 with the changed preferences", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Preferences).NotTo(BeNil())
					Expect(got.Preferences.Currency).To(Equal("GBP"))
					Expect(got.Preferences.ShowPriceToMe).To(BeFalse())
					Expect(got.Preferences.ShowPriceToOthers).To(BeTrue())
				})
			})
		})

		Context("given a PUT that omits preferences", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 and resets preferences to defaults", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Preferences).NotTo(BeNil())
					Expect(got.Preferences.Currency).To(Equal("USD"))
					Expect(got.Preferences.ShowPriceToMe).To(BeTrue())
					Expect(got.Preferences.ShowPriceToOthers).To(BeFalse())
				})
			})
		})

		Context("given a PUT with a blank discord_username", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true, "discord_username": "   "}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 200 with discord_username cleared, not empty", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.DiscordUsername).To(BeNil())
				})
			})
		})

		Context("given a PUT with a malformed discord_username", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true, "discord_username": "na@me"}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 naming discord_username", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(invalidParamNames(decodeProblem(resp))).To(ContainElement("discord_username"))
				})
			})
		})

		Context("given a PUT with an unknown top-level field", func() {
			When("updating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true, "show_price_to_others": true}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a validation-failed problem", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

					problem := decodeProblem(resp)
					Expect(problem.Type).To(Equal("https://mykeebs.info/errors/bad-request"))
					Expect(problem.InvalidParams).NotTo(BeEmpty())
				})
			})
		})

		Context("given the new username is taken by another user", func() {
			var (
				otherID   string
				otherUser string
			)

			BeforeEach(func(ctx SpecContext) {
				var err error
				_, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				otherUser = "u" + uuid.NewString()[:8]
				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: otherUser, Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, otherUser)).To(Succeed())
			})

			When("updating to that username", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Update(ctx, ownerID, ownerToken, fmt.Sprintf(
						`{"username": %q, "discoverable": true}`, otherUser))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 409 with the username-unavailable type", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusConflict))
					Expect(decodeProblem(resp).Type).
						To(Equal("https://mykeebs.info/errors/username-unavailable"))
				})
			})
		})
	})

	Context("given the caller has no profile", func() {
		When("updating the profile", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Update(ctx, ownerID, ownerToken,
					fmt.Sprintf(`{"username": %q}`, username))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	Context("given the path identifier is not the caller", func() {
		When("updating the profile", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Update(ctx, "user-someone-"+uuid.NewString(), ownerToken,
					fmt.Sprintf(`{"username": %q}`, username))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404, not 403, to avoid revealing whose profile it is", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
