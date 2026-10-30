package keyboards_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Managing a keyboard's plates and PCBs", func() {
	var (
		resp       *http.Response
		client     *api.KeyboardsClient
		ownerID    string
		ownerToken string
		keyboardID string
	)

	// readKeyboard GETs the keyboard as its owner and decodes its parts.
	readKeyboard := func(ctx SpecContext) keyboardWithParts {
		getResp, err := client.Get(ctx, ownerID, keyboardID, ownerToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(getResp.StatusCode).To(Equal(http.StatusOK))

		var got keyboardWithParts
		Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
		return got
	}

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewKeyboardsClient()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		keyboardID = "keyboard-parts-" + uuid.NewString()
		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given the caller owns a keyboard with one plate and one PCB", func() {
		Context("given a plate with its own purchase", func() {
			When("adding a plate", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.CreatePart(ctx, ownerID, keyboardID, "plates", ownerToken,
						`{"material":"PC","thickness":1.5,"purchase":{"vendor":"`+approvedVendor+`","price":25}}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("adds it after the existing plate with its own id, and counts its price", func(ctx SpecContext) {
					By("returning 201 with a server-generated id")
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))
					var created struct {
						ID       string `json:"id"`
						Material string `json:"material"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&created)).To(Succeed())
					Expect(created.ID).NotTo(BeEmpty())
					Expect(created.ID).NotTo(Equal(db.SeededPlateID))
					Expect(created.Material).To(Equal("PC"))

					By("listing it after the keyboard's existing plate")
					got := readKeyboard(ctx)
					Expect(got.Plates).To(HaveLen(2))
					Expect(got.Plates[0].ID).To(Equal(db.SeededPlateID))
					Expect(got.Plates[1].ID).To(Equal(created.ID))
					Expect(*got.Plates[1].Thickness).To(Equal(1.5))

					By("adding its price to total_cost")
					Expect(got.TotalCost).NotTo(BeNil())
					Expect(*got.TotalCost).To(Equal(394.99))
				})
			})
		})

		Context("given a PCB", func() {
			When("adding a PCB", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.CreatePart(ctx, ownerID, keyboardID, "pcbs", ownerToken,
						`{"firmware":"QMK/VIA","assembly":"Solder","connectivity":"Wired"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("adds it after the existing PCB", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					got := readKeyboard(ctx)
					Expect(got.PCBs).To(HaveLen(2))
					Expect(got.PCBs[0].ID).To(Equal(db.SeededPCBID))
					Expect(got.PCBs[1].ID).NotTo(BeEmpty())
				})
			})
		})

		Context("given a plate whose material isn't an approved value", func() {
			When("adding a plate", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.CreatePart(ctx, ownerID, keyboardID, "plates", ownerToken, `{"material":"NotAMaterial"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 naming material, and adds nothing", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

					var problem struct {
						InvalidParams []struct {
							Name string `json:"name"`
						} `json:"invalid_params"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&problem)).To(Succeed())
					Expect(problem.InvalidParams).To(ConsistOf(HaveField("Name", "material")))

					Expect(readKeyboard(ctx).Plates).To(HaveLen(1))
				})
			})
		})

		Context("given no build uses the plate", func() {
			When("deleting the plate", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.DeletePart(ctx, ownerID, keyboardID, "plates", db.SeededPlateID, ownerToken, "")
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 204 and removes it", func(ctx SpecContext) {
					Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
					Expect(readKeyboard(ctx).Plates).To(BeEmpty())
				})
			})
		})

		Context("given a build uses the keyboard's plate and PCB", func() {
			var (
				builds  *api.BuildsClient
				buildID string
			)

			BeforeEach(func(ctx SpecContext) {
				builds = api.NewBuildsClient()
				buildID = "keyboard-parts-build-" + uuid.NewString()
				Expect(db.SeedBuildWithParts(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			Context("given changed details for the plate", func() {
				When("updating the plate", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.UpdatePart(ctx, ownerID, keyboardID, "plates", db.SeededPlateID, ownerToken,
							`{"material":"AL","color":"Silver"}`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("keeps the plate's id, so the build shows its new details", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusOK))

						buildResp, err := builds.Get(ctx, ownerID, buildID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(buildResp.StatusCode).To(Equal(http.StatusOK))

						var got struct {
							Plate *struct {
								ID       string  `json:"id"`
								Material string  `json:"material"`
								Color    *string `json:"color"`
							} `json:"plate"`
						}
						Expect(json.NewDecoder(buildResp.Body).Decode(&got)).To(Succeed())
						Expect(got.Plate).NotTo(BeNil())
						Expect(got.Plate.ID).To(Equal(db.SeededPlateID))
						Expect(got.Plate.Material).To(Equal("AL"))
						Expect(*got.Plate.Color).To(Equal("Silver"))
					})
				})
			})

			Context("given on_delete is omitted (defaults to block)", func() {
				When("deleting the plate", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeletePart(ctx, ownerID, keyboardID, "plates", db.SeededPlateID, ownerToken, "")
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns 409 listing the build, and keeps the plate", func(ctx SpecContext) {
						Expect(resp.StatusCode).To(Equal(http.StatusConflict))

						var problem struct {
							BlockingBuildIDs []string `json:"blocking_build_ids"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&problem)).To(Succeed())
						Expect(problem.BlockingBuildIDs).To(ConsistOf(buildID))

						Expect(readKeyboard(ctx).Plates).To(ConsistOf(HaveField("ID", db.SeededPlateID)))
					})
				})
			})

			Context("given on_delete is cascade", func() {
				When("deleting the PCB", func() {
					BeforeEach(func(ctx SpecContext) {
						var err error
						resp, err = client.DeletePart(ctx, ownerID, keyboardID, "pcbs", db.SeededPCBID, ownerToken, "cascade")
						Expect(err).NotTo(HaveOccurred())
					})

					It("deletes the build along with the PCB", func(ctx SpecContext) {
						By("returning 200 listing the deleted build")
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						var result struct {
							DeletedBuildIDs []string `json:"deleted_build_ids"`
						}
						Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
						Expect(result.DeletedBuildIDs).To(ConsistOf(buildID))

						By("removing the build")
						buildResp, err := builds.Get(ctx, ownerID, buildID, ownerToken)
						Expect(err).NotTo(HaveOccurred())
						Expect(buildResp.StatusCode).To(Equal(http.StatusNotFound))

						By("removing the PCB")
						Expect(readKeyboard(ctx).PCBs).To(BeEmpty())
					})
				})
			})
		})
	})

	Context("given another user is the caller", func() {
		var otherToken string

		BeforeEach(func(ctx SpecContext) {
			var err error
			otherToken, _, err = api.NewAuthIdentity(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		When("adding a plate to the owner's keyboard", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.CreatePart(ctx, ownerID, keyboardID, "plates", otherToken, `{"material":"PC"}`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404 and adds nothing", func(ctx SpecContext) {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(readKeyboard(ctx).Plates).To(HaveLen(1))
			})
		})
	})
})
