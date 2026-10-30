package profiles_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a profile's avatar", func() {
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
				Username:     username,
				Discoverable: true,
			})).To(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			Expect(db.DeleteProfile(ctx, ownerID, username)).To(Succeed())
		})

		Context("given the profile has an avatar", func() {
			BeforeEach(func(ctx SpecContext) {
				setResp, err := client.SetImage(ctx, ownerID, ownerToken, api.ImageUploadBody(approvedImageContentType))
				Expect(err).NotTo(HaveOccurred())
				Expect(setResp.StatusCode).To(Equal(http.StatusCreated))
			})

			Context("given the caller is the owner", func() {
				When("deleting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteImage(ctx, ownerID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 204 and the avatar is gone from a follow-up GetProfile", func(ctx SpecContext) {
						By("returning 204 No Content")
						Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

						By("no longer showing an avatar")
						getResp, err := client.Get(ctx, ownerID, ownerToken)
						Expect(err).NotTo(HaveOccurred())

						var p struct {
							Avatar *struct {
								URL string `json:"url"`
							} `json:"avatar"`
						}
						Expect(json.NewDecoder(getResp.Body).Decode(&p)).To(Succeed())
						Expect(p.Avatar).To(BeNil())
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

				When("deleting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteImage(ctx, ownerID, token)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 404, not 403, to avoid revealing whose profile it is", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
						Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
					})
				})
			})

			Context("given the caller is anonymous", func() {
				When("deleting the avatar", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeleteImage(ctx, ownerID, "")
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 401", func() {
						Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					})
				})
			})
		})

		Context("given the profile has no avatar set", func() {
			When("deleting the avatar", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeleteImage(ctx, ownerID, ownerToken)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204, idempotently", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
				})
			})
		})
	})

	Context("given the caller has no profile", func() {
		When("deleting the avatar", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.DeleteImage(ctx, ownerID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
