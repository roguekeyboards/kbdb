package keyboards_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Deleting a keyboard that is still referenced by a build", func() {
	var (
		resp         *http.Response
		keyboards    *api.KeyboardsClient
		builds       *api.BuildsClient
		ownerID      string
		ownerToken   string
		keyboardID   string
		buildID      string
		keyboardGone bool
		buildGone    bool
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		keyboards = api.NewKeyboardsClient()
		builds = api.NewBuildsClient()
		keyboardGone = false
		buildGone = false

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		keyboardID = "cascade-keyboard-" + uuid.NewString()
		buildID = "cascade-build-" + uuid.NewString()

		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		Expect(db.SeedBuild(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		if !buildGone {
			Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
		}
		if !keyboardGone {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		}
	})

	Context("given on_delete is omitted (defaults to block)", func() {
		When("deleting the keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keyboards.DeleteWithOnDelete(ctx, ownerID, keyboardID, ownerToken, "")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 409 with the blocking build id, and both still exist", func(ctx SpecContext) {
				By("returning 409 with blocking_build_ids")
				Expect(resp.StatusCode).To(Equal(http.StatusConflict))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var body struct {
					BlockingBuildIDs []string `json:"blocking_build_ids"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body.BlockingBuildIDs).To(ContainElement(buildID))

				By("the keyboard still existing")
				getKb, err := keyboards.Get(ctx, ownerID, keyboardID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getKb.StatusCode).To(Equal(http.StatusOK))

				By("the build still existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusOK))
			})
		})
	})

	Context("given on_delete=block explicitly", func() {
		When("deleting the keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keyboards.DeleteWithOnDelete(ctx, ownerID, keyboardID, ownerToken, "block")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 409, same as the default", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusConflict))
			})
		})
	})

	Context("given on_delete=cascade", func() {
		When("deleting the keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keyboards.DeleteWithOnDelete(ctx, ownerID, keyboardID, ownerToken, "cascade")
				Expect(err).NotTo(HaveOccurred())
				if resp.StatusCode == http.StatusOK {
					keyboardGone = true
					buildGone = true
				}
			})

			It("deletes the keyboard and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				By("returning 200 with deleted_build_ids")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var body struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body.DeletedBuildIDs).To(ContainElement(buildID))

				By("the keyboard no longer existing")
				getKb, err := keyboards.Get(ctx, ownerID, keyboardID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getKb.StatusCode).To(Equal(http.StatusNotFound))

				By("the build no longer existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	Context("given on_delete is an invalid value", func() {
		When("deleting the keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keyboards.DeleteWithOnDelete(ctx, ownerID, keyboardID, ownerToken, "bogus")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 400", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
	})
})
