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

var _ = Describe("Deleting a keycap set with a kit that is still referenced by a build", func() {
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
		setGone    bool
		buildGone  bool
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		keycapSets = api.NewKeycapSetsClient()
		builds = api.NewBuildsClient()
		setGone = false
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
		if !setGone {
			Expect(db.DeleteKeycapSet(ctx, ownerID, setID)).To(Succeed())
		}
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given on_delete is omitted (defaults to block)", func() {
		When("deleting the set", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteWithOnDelete(ctx, ownerID, setID, ownerToken, "")
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

				By("the set still existing")
				getSet, err := keycapSets.Get(ctx, ownerID, setID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getSet.StatusCode).To(Equal(http.StatusOK))

				By("the build still existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusOK))
			})
		})
	})

	Context("given on_delete=block explicitly", func() {
		When("deleting the set", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteWithOnDelete(ctx, ownerID, setID, ownerToken, "block")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 409, same as the default", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusConflict))
			})
		})
	})

	Context("given on_delete=cascade", func() {
		When("deleting the set", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteWithOnDelete(ctx, ownerID, setID, ownerToken, "cascade")
				Expect(err).NotTo(HaveOccurred())
				if resp.StatusCode == http.StatusOK {
					setGone = true
					buildGone = true
				}
			})

			It("deletes the set and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				By("returning 200 with deleted_build_ids")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var body struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body.DeletedBuildIDs).To(ContainElement(buildID))

				By("the set no longer existing")
				getSet, err := keycapSets.Get(ctx, ownerID, setID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getSet.StatusCode).To(Equal(http.StatusNotFound))

				By("the build no longer existing")
				getBuild, err := builds.Get(ctx, ownerID, buildID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	Context("given on_delete is an invalid value", func() {
		When("deleting the set", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteWithOnDelete(ctx, ownerID, setID, ownerToken, "bogus")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 400", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
	})
})

var _ = Describe("Deleting a keycap set with multiple kits referenced by different builds", func() {
	var (
		resp       *http.Response
		keycapSets *api.KeycapSetsClient
		builds     *api.BuildsClient
		ownerID    string
		ownerToken string
		keyboardID string
		setID      string
		kitID1     string
		kitID2     string
		buildID1   string
		buildID2   string
		setGone    bool
		build1Gone bool
		build2Gone bool
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		keycapSets = api.NewKeycapSetsClient()
		builds = api.NewBuildsClient()
		setGone = false
		build1Gone = false
		build2Gone = false

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		keyboardID = "cascade-multi-fixture-keyboard-" + uuid.NewString()
		setID = "cascade-multi-keycap-set-" + uuid.NewString()
		kitID1 = "cascade-kit-1-" + uuid.NewString()
		kitID2 = "cascade-kit-2-" + uuid.NewString()
		buildID1 = "cascade-build-1-" + uuid.NewString()
		buildID2 = "cascade-build-2-" + uuid.NewString()

		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		Expect(db.SeedKeycapSetWithKits(ctx, ownerID, setID, []string{kitID1, kitID2}, "private")).To(Succeed())
		Expect(db.SeedBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID1, keyboardID, setID, kitID1, "private")).To(Succeed())
		Expect(db.SeedBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID2, keyboardID, setID, kitID2, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		if !build1Gone {
			Expect(db.DeleteBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID1, keyboardID, setID, kitID1)).To(Succeed())
		}
		if !build2Gone {
			Expect(db.DeleteBuildWithKeycapKitAndKeyboard(ctx, ownerID, buildID2, keyboardID, setID, kitID2)).To(Succeed())
		}
		if !setGone {
			Expect(db.DeleteKeycapSet(ctx, ownerID, setID)).To(Succeed())
		}
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given on_delete=cascade", func() {
		When("deleting the set", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = keycapSets.DeleteWithOnDelete(ctx, ownerID, setID, ownerToken, "cascade")
				Expect(err).NotTo(HaveOccurred())
				if resp.StatusCode == http.StatusOK {
					setGone = true
					build1Gone = true
					build2Gone = true
				}
			})

			It("finds and deletes both builds via a single begins_with prefix query", func(ctx SpecContext) {
				By("returning 200 with both deleted build ids")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var body struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body.DeletedBuildIDs).To(ConsistOf(buildID1, buildID2))

				By("the set no longer existing")
				getSet, err := keycapSets.Get(ctx, ownerID, setID, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getSet.StatusCode).To(Equal(http.StatusNotFound))

				By("both builds no longer existing")
				getBuild1, err := builds.Get(ctx, ownerID, buildID1, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild1.StatusCode).To(Equal(http.StatusNotFound))

				getBuild2, err := builds.Get(ctx, ownerID, buildID2, ownerToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(getBuild2.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})
})
