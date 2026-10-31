package profiles_test

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

type profileBody struct {
	Username        string  `json:"username"`
	Discoverable    *bool   `json:"discoverable"`
	DiscordUsername *string `json:"discord_username"`
	Bio             *string `json:"bio"`
	Links           *[]struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"links"`
	Avatar *struct {
		URL string `json:"url"`
	} `json:"avatar"`
	UserID      string `json:"user_id"`
	Preferences *struct {
		Currency          string `json:"currency"`
		ShowPriceToMe     bool   `json:"show_price_to_me"`
		ShowPriceToOthers bool   `json:"show_price_to_others"`
	} `json:"preferences"`
}

var _ = Describe("Getting a profile", func() {
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

	Context("given a discoverable profile exists", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:        username,
				Discoverable:    true,
				DiscordUsername: "alice_kb",
				Bio:             "keebs enjoyer",
				Links: []map[string]string{
					{"name": "Twitch", "url": "https://twitch.tv/alice"},
				},
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		Context("given the identifier is the owner's user id", func() {
			When("getting the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the full profile including the IdP subject as user_id", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					body, err := io.ReadAll(resp.Body)
					Expect(err).NotTo(HaveOccurred())

					var got profileBody
					Expect(json.Unmarshal(body, &got)).To(Succeed())
					Expect(got.Username).To(Equal(username))
					Expect(got.Bio).NotTo(BeNil())
					Expect(*got.Bio).To(Equal("keebs enjoyer"))
					Expect(got.Links).NotTo(BeNil())
					Expect(*got.Links).To(HaveLen(1))

					By("returning the IdP subject as user_id so the caller can address the collection routes")
					Expect(got.UserID).To(Equal(ownerID))

					By("omitting the avatar key entirely rather than emitting it as null")
					var raw map[string]json.RawMessage
					Expect(json.Unmarshal(body, &raw)).To(Succeed())
					Expect(raw).NotTo(HaveKey("avatar"))
				})
			})
		})

		Context("given the identifier is the username", func() {
			When("getting the profile anonymously", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, username, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("resolves the username and returns the same profile", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Username).To(Equal(username))

					By("omitting preferences, which only the owner sees")
					Expect(got.Preferences).To(BeNil())
				})
			})
		})
	})

	Context("given a profile exists with no stored preferences (predates this field)", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:     username,
				Discoverable: true,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns default preferences rather than empty/false values", func() {
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
	})

	Context("given a non-discoverable profile exists", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username:     username,
				Discoverable: false,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		Context("given the caller is the owner", func() {
			When("getting the profile by its own user id", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, ownerID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the profile", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				})
			})
		})

		Context("given the caller is a different user", func() {
			var otherToken string

			BeforeEach(func(ctx SpecContext) {
				var err error
				otherToken, _, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			When("getting the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, username, otherToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404, not 403, to avoid revealing it exists", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the caller is anonymous", func() {
			When("getting the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Get(ctx, username, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 404", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				})
			})
		})
	})

	Context("given no profile matches the identifier", func() {
		When("getting by a bogus user id", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, "user-nobody-"+uuid.NewString(), ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		When("getting by a bogus username", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Get(ctx, "ghost"+uuid.NewString()[:8], ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})
})
