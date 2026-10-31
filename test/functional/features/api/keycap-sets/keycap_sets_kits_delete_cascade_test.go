package keycapsets_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Deleting a keycap kit that is still referenced by a build", func() {
	var (
		resp       *http.Response
		keycapSets *api.KeycapSetsClient
		builds     *api.BuildsClient
		ownerID    string
		ownerToken string
		keyboardID string
		setID      string
		kitID      string
		buildID    string
		kitGone    bool
		buildGone  bool
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		keycapSets = api.NewKeycapSetsClient()
		builds = api.NewBuildsClient()
		kitGone = false
		buildGone = false

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		keyboardID = "cascade-fixture-keyboard-" + uuid.NewString()
		setID = "cascade-keycap-set-" + uuid.NewString()
		kitID = "cascade-kit-" + uuid.NewString()
		buildID = "cascade-build-" + uuid.NewString()

		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		Expect(db.SeedKeycapSetWithKit(ctx, ownerID, setID, kitID, "private")).To(Succeed())
		Expect(db.SeedBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID, keyboardID, setID, kitID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		if !buildGone {
			Expect(db.DeleteBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID, keyboardID, setID, kitID)).To(Succeed())
		}
		if !kitGone {
			Expect(db.DeleteKeycapSet(ctx, ownerID, setID)).To(Succeed())
		}
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given on_delete is omitted (defaults to block)", func() {
		When("deleting the kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteKitWithOnDelete(ctx, ownerID, setID, kitID, ownerToken, "")
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

				By("the kit still existing")
				getSet, err := keycapSets.Get(ctx, ownerID, setID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getSet.StatusCode).To(Equal(http.StatusOK))

				var setBody struct {
					Kits []struct {
						KitID string `json:"kit_id"`
					} `json:"kits"`
				}
				Expect(json.NewDecoder(getSet.Body).Decode(&setBody)).To(Succeed())
				Expect(setBody.Kits).To(HaveLen(1))
				Expect(setBody.Kits[0].KitID).To(Equal(kitID))

				By("the build still existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusOK))
			})
		})
	})

	Context("given on_delete=block explicitly", func() {
		When("deleting the kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteKitWithOnDelete(ctx, ownerID, setID, kitID, ownerToken, "block")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 409, same as the default", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusConflict))
			})
		})
	})

	Context("given on_delete=cascade", func() {
		When("deleting the kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteKitWithOnDelete(ctx, ownerID, setID, kitID, ownerToken, "cascade")
				Expect(err).NotTo(HaveOccurred())
				if resp.StatusCode == http.StatusOK {
					kitGone = true
					buildGone = true
				}
			})

			It("deletes the kit and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				By("returning 200 with deleted_build_ids")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var body struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body.DeletedBuildIDs).To(ContainElement(buildID))

				By("the kit no longer existing on the set")
				getSet, err := keycapSets.Get(ctx, ownerID, setID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getSet.StatusCode).To(Equal(http.StatusOK))

				var setBody struct {
					Kits []struct {
						KitID string `json:"kit_id"`
					} `json:"kits"`
				}
				Expect(json.NewDecoder(getSet.Body).Decode(&setBody)).To(Succeed())
				Expect(setBody.Kits).To(BeEmpty())

				By("the build no longer existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	Context("given on_delete is an invalid value", func() {
		When("deleting the kit", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteKitWithOnDelete(ctx, ownerID, setID, kitID, ownerToken, "bogus")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 400", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
	})
})
