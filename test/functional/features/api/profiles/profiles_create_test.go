package profiles_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

type problemBody struct {
	Type          string `json:"type"`
	Detail        string `json:"detail"`
	InvalidParams []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"invalid_params"`
}

func decodeProblem(resp *http.Response) problemBody {
	GinkgoHelper()
	var p problemBody
	Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
	return p
}

func invalidParamNames(p problemBody) []string {
	names := make([]string, len(p.InvalidParams))
	for i, ip := range p.InvalidParams {
		names[i] = ip.Name
	}
	return names
}

var _ = Describe("Creating a profile", func() {
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

	Context("given the caller has no profile", func() {
		AfterEach(func(ctx SpecContext) {
			// Best-effort: only the "valid input" case actually creates one.
			_ = db.DeleteProfile(ctx, ownerID, username)
		})

		Context("given valid input", func() {
			var body string

			BeforeEach(func() {
				body = fmt.Sprintf(`{
					"username": %q,
					"discoverable": true,
					"discord_username": "alice_kb",
					"bio": "keebs enjoyer",
					"links": [{"name": "Twitch", "url": "https://twitch.tv/alice"}]
				}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 and the created profile", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Username).To(Equal(username))

					By("returning the IdP subject as user_id")
					Expect(got.UserID).To(Equal(ownerID))
				})

				It("is then resolvable by id and by username", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					byID, err := client.Get(ctx, ownerID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
					Expect(byID.StatusCode).To(Equal(http.StatusOK))

					byName, err := client.Get(ctx, username, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(byName.StatusCode).To(Equal(http.StatusOK))
				})
			})
		})

		Context("given a username containing a period and a hyphen", func() {
			var body string

			BeforeEach(func() {
				username = "my.kb-" + uuid.NewString()[:8]
				body = fmt.Sprintf(`{"username": %q, "discoverable": true}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 and is then resolvable by that username", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					byName, err := client.Get(ctx, username, "")
					Expect(err).NotTo(HaveOccurred())
					Expect(byName.StatusCode).To(Equal(http.StatusOK))

					var got profileBody
					Expect(json.NewDecoder(byName.Body).Decode(&got)).To(Succeed())
					Expect(got.Username).To(Equal(username))
				})
			})
		})

		Context("given preferences are omitted", func() {
			var body string

			BeforeEach(func() {
				body = fmt.Sprintf(`{"username": %q}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 with default preferences", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Preferences).NotTo(BeNil())
					Expect(got.Preferences.Currency).To(Equal("USD"))
					Expect(got.Preferences.ShowPriceToMe).To(BeTrue())
					Expect(got.Preferences.ShowPriceToOthers).To(BeFalse())
				})
			})
		})

		Context("given explicit preferences", func() {
			var body string

			BeforeEach(func() {
				body = fmt.Sprintf(`{
					"username": %q,
					"preferences": {"currency": "EUR", "show_price_to_me": false, "show_price_to_others": true}
				}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 with the given preferences", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Preferences).NotTo(BeNil())
					Expect(got.Preferences.Currency).To(Equal("EUR"))
					Expect(got.Preferences.ShowPriceToMe).To(BeFalse())
					Expect(got.Preferences.ShowPriceToOthers).To(BeTrue())
				})
			})
		})

		Context("given a lowercase currency", func() {
			var body string

			BeforeEach(func() {
				body = fmt.Sprintf(`{"username": %q, "preferences": {"currency": "eur"}}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 with currency uppercased", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.Preferences).NotTo(BeNil())
					Expect(got.Preferences.Currency).To(Equal("EUR"))
				})
			})
		})

		Context("given a blank discord_username", func() {
			var body string

			BeforeEach(func() {
				body = fmt.Sprintf(`{"username": %q, "discoverable": true, "discord_username": "   "}`, username)
			})

			When("creating the profile", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, body)
					Expect(err).NotTo(HaveOccurred())
				})

				It("creates the profile with discord_username absent, not empty", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got profileBody
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.DiscordUsername).To(BeNil())
				})
			})
		})

		Context("given a username already claimed by another user", func() {
			var otherID string

			BeforeEach(func(ctx SpecContext) {
				var err error
				_, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(db.SeedProfile(ctx, otherID, db.SeedProfileOptions{
					Username: username, Discoverable: true,
				})).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteProfile(ctx, otherID, username)).To(Succeed())
			})

			When("creating a profile with that username", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						fmt.Sprintf(`{"username": %q}`, username))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 409 with the username-unavailable type", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusConflict))
					Expect(decodeProblem(resp).Type).
						To(Equal("https://mykeebs.info/errors/username-unavailable"))
				})
			})
		})

		DescribeTable("given the input fails validation",
			func(ctx SpecContext, body string, wantParam string) {
				var err error
				resp, err = client.Create(ctx, ownerID, ownerToken, body)
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(invalidParamNames(decodeProblem(resp))).To(ContainElement(wantParam))
			},
			Entry("username fails the pattern",
				`{"username": "AB"}`, "username"),
			Entry("username has a leading period",
				`{"username": ".lead"}`, "username"),
			Entry("username has a trailing underscore",
				`{"username": "trail_"}`, "username"),
			Entry("username has consecutive periods",
				`{"username": "a..b"}`, "username"),
			Entry("username is over 32 characters",
				`{"username": "`+strings.Repeat("x", 33)+`"}`, "username"),
			Entry("a link url is http not https",
				`{"username": "aaa", "links": [{"name": "s", "url": "http://x.example"}]}`,
				"links[0].url"),
			Entry("a 6th link",
				`{"username": "aaa", "links": [`+strings.Repeat(`{"name":"s","url":"https://x.example"},`, 5)+`{"name":"s","url":"https://x.example"}]}`,
				"links"),
			Entry("discord_username over 32",
				`{"username": "aaa", "discord_username": "`+strings.Repeat("x", 33)+`"}`,
				"discord_username"),
			Entry("discord_username has an invalid character",
				`{"username": "aaa", "discord_username": "na@me"}`,
				"discord_username"),
			Entry("bio over 500",
				`{"username": "aaa", "bio": "`+strings.Repeat("x", 501)+`"}`,
				"bio"),
			Entry("currency is not 3 letters",
				`{"username": "aaa", "preferences": {"currency": "US"}}`,
				"preferences.currency"),
		)
	})

	Context("given the caller already has a profile", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(db.SeedProfile(ctx, ownerID, db.SeedProfileOptions{
				Username: username, Discoverable: false,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		When("creating another profile", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Create(ctx, ownerID, ownerToken,
					fmt.Sprintf(`{"username": "other%s"}`, uuid.NewString()[:6]))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 409 with the generic conflict type", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusConflict))
				Expect(decodeProblem(resp).Type).
					To(Equal("https://mykeebs.info/errors/conflict"))
			})
		})
	})

	Context("given the path userId is not the caller", func() {
		When("creating a profile", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Create(ctx, "user-someone-"+uuid.NewString(), ownerToken,
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
