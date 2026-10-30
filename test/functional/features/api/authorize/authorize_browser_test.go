package authorize_test

import (
	"net/url"
	"time"

	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support"
	"github.com/rogueserenity/kbdb/test/functional/support/browser"
)

const authorizeQuery = "client_id=client-test&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback" +
	"&response_type=code&scope=openid&state=state-test&code_challenge=challenge-test&code_challenge_method=S256"

var _ = Describe("Using the IdP consent page in a browser", func() {
	var session *browser.Session

	newSession := func(opts browser.Options) {
		var err error
		session, err = chromium.NewSession(opts)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(session.Close)
	}

	waitForText := func(text string) {
		Expect(session.Page.GetByText(text).First().WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(float64((15 * time.Second).Milliseconds())),
		})).To(Succeed())
	}

	Context("given no IdP session", func() {
		BeforeEach(func() { newSession(browser.Options{}) })

		When("the page is opened", func() {
			BeforeEach(func() {
				Expect(session.Open("/authorize")).To(Succeed())
				waitForText("Continue with Discord")
			})

			It("offers sign-in with Discord, loading only from the stack, the IdP and mykeebs.com", func() {
				By("running without script errors")
				Expect(session.PageErrors()).To(BeEmpty())
				Expect(session.Page.GetByText("Sign in to continue").IsVisible()).To(BeTrue())

				By("loading the IdP SDK from the stack, intact")
				Expect(session.Page.Evaluate("typeof window.stytch")).To(Equal("object"))

				By("loading the brand fonts")
				Expect(session.Page.Evaluate(`document.fonts.load('1em Inter').then(f => f.length > 0 && f[0].status)`)).To(Equal("loaded"))

				By("loading nothing from any other origin")
				Expect(session.Elsewhere()).To(BeEmpty())
			})
		})

		When("the user continues with Discord", func() {
			BeforeEach(func() {
				Expect(session.Open("/authorize")).To(Succeed())
				waitForText("Continue with Discord")
				Expect(session.Page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Continue with Discord"}).Click()).To(Succeed())
				waitForText("IdP OAuth start")
			})

			It("starts the IdP's Discord OAuth flow, returning to this page", func() {
				starts := session.IDPRequests("/v1/public/oauth/discord/start")
				Expect(starts).To(HaveLen(1))
				q := starts[0].URL.Query()
				Expect(q.Get("public_token")).To(Equal(browser.PublicToken))
				Expect(q.Get("login_redirect_url")).To(Equal(support.BaseURL() + "/authorize"))
				Expect(q.Get("signup_redirect_url")).To(Equal(support.BaseURL() + "/authorize"))
			})
		})
	})

	Context("given an IdP session", func() {
		BeforeEach(func() { newSession(browser.Options{SignedIn: true}) })

		Context("given an OAuth authorize request", func() {
			When("the page is opened", func() {
				BeforeEach(func() {
					Expect(session.Open("/authorize?" + authorizeQuery)).To(Succeed())
					waitForText("Allow")
				})

				It("asks for consent on behalf of the signed-in user", func() {
					By("running without script errors")
					Expect(session.PageErrors()).To(BeEmpty())

					By("re-checking the session with the IdP")
					Expect(session.IDPRequests("/sdk/v1/sessions/authenticate")).To(ContainElement(
						HaveField("Body", HaveKeyWithValue("session_duration_minutes", BeNumerically("==", 60)))))

					By("naming the user and offering to switch account")
					Expect(session.Page.Locator("#status").InnerText()).To(Equal("Signed in as Test User"))
					Expect(session.Page.Locator("#switch-account").IsVisible()).To(BeTrue())

					By("passing the authorize request to the IdP")
					starts := session.IDPRequests("/sdk/v1/idp/oauth/authorize/start")
					Expect(starts).NotTo(BeEmpty())
					Expect(starts[0].Body).To(And(
						HaveKeyWithValue("client_id", "client-test"),
						HaveKeyWithValue("redirect_uri", "https://example.com/callback"),
						HaveKeyWithValue("state", "state-test"),
						HaveKeyWithValue("code_challenge", "challenge-test"),
					))

					By("naming the client asking for consent")
					Expect(session.Page.GetByText("Test Client").First().IsVisible()).To(BeTrue())
				})
			})
		})

		Context("given an OIDC logout request", func() {
			When("the page is opened", func() {
				BeforeEach(func() {
					Expect(session.Open("/authorize?client_id=client-test&state=state-test" +
						"&post_logout_redirect_uri=" + url.QueryEscape("https://example.com/signed-out"))).To(Succeed())
					waitForText("Log out?")
				})

				It("passes the logout request to the IdP and asks to confirm", func() {
					By("running without script errors")
					Expect(session.PageErrors()).To(BeEmpty())

					By("passing the logout request to the IdP")
					starts := session.IDPRequests("/sdk/v1/oauth/logout/start")
					Expect(starts).NotTo(BeEmpty())
					Expect(starts[0].Body).To(And(
						HaveKeyWithValue("client_id", "client-test"),
						HaveKeyWithValue("post_logout_redirect_uri", "https://example.com/signed-out"),
					))
				})
			})
		})
	})

	Context("given Discord sent the user back during an OAuth authorize request", func() {
		BeforeEach(func() {
			newSession(browser.Options{})
			Expect(session.Open("/authorize?" + authorizeQuery)).To(Succeed())
			waitForText("Continue with Discord")
		})

		When("the page is opened with the IdP's token", func() {
			BeforeEach(func() {
				Expect(session.Open("/authorize?token=oauth-token-test&stytch_token_type=oauth")).To(Succeed())
				waitForText("Allow")
			})

			It("signs in and resumes the original authorize request", func() {
				By("exchanging the token with the IdP")
				Expect(session.IDPRequests("/sdk/v1/oauth/authenticate")).To(ContainElement(
					HaveField("Body", HaveKeyWithValue("token", "oauth-token-test"))))

				By("restoring the authorize request's URL")
				Expect(session.Page.URL()).To(HaveSuffix("/authorize?" + authorizeQuery))

				By("passing the authorize request to the IdP")
				starts := session.IDPRequests("/sdk/v1/idp/oauth/authorize/start")
				Expect(starts).NotTo(BeEmpty())
				Expect(starts[0].Body).To(HaveKeyWithValue("client_id", "client-test"))
			})
		})
	})

	Context("given mykeebs.com's fonts can't be reached", func() {
		BeforeEach(func() { newSession(browser.Options{BlockFonts: true}) })

		When("the page is opened", func() {
			BeforeEach(func() {
				Expect(session.Open("/authorize")).To(Succeed())
				waitForText("Continue with Discord")
			})

			It("still offers sign-in with Discord", func() {
				Expect(session.PageErrors()).To(BeEmpty())
			})
		})
	})
})
