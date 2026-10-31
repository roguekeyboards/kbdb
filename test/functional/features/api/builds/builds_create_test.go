package builds_test

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Creating a build", func() {
	var (
		resp       *http.Response
		client     *api.BuildsClient
		ownerID    string
		ownerToken string
		keyboardID string
		createdID  string
	)

	BeforeEach(func(ctx SpecContext) {
		resp = nil
		client = api.NewBuildsClient()
		createdID = ""
		keyboardID = "build-fixture-keyboard-" + uuid.NewString()

		var err error
		ownerToken, ownerID, err = api.NewAuthIdentity(ctx)
		Expect(err).NotTo(HaveOccurred())

		// A build references a keyboard, so seed one directly rather than
		// through the API - keeps this spec focused on the build create
		// route itself.
		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		if createdID != "" {
			Expect(db.DeleteBuild(ctx, ownerID, createdID, keyboardID)).To(Succeed())
		}
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	// captureCreatedID is called from BeforeEach, right after a successful
	// Create response is decoded - not from inside an It block after that
	// spec's own assertions. An assertion failing partway through an It
	// must not skip capturing this and leak the created build.
	captureCreatedID := func(r *http.Response) {
		var got struct {
			ID string `json:"id"`
		}
		Expect(json.NewDecoder(r.Body).Decode(&got)).To(Succeed())
		createdID = got.ID
	}

	Context("given the caller is the owner", func() {
		Context("given a valid request body", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusCreated {
						captureCreatedID(resp)
					}
				})

				It("creates the build", func() {
					By("returning 201 Created")
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					By("returning a server-generated id")
					Expect(createdID).NotTo(BeEmpty())
				})
			})
		})

		Context("given every open-vocabulary field has an approved lookup value", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"stabs":{"name":"`+approvedStabilizer+`","mount_type":"`+approvedStabilizerMount+`"},`+
							`"case_mount_type":{"type":"`+approvedCaseMountType+`","durometer":"`+approvedDurometer+`"}}`)
					Expect(err).NotTo(HaveOccurred())
					if resp.StatusCode == http.StatusCreated {
						captureCreatedID(resp)
					}
				})

				It("creates the build", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))
					Expect(createdID).NotTo(BeEmpty())
				})
			})
		})

		Context("given an open-vocabulary field has an unapproved value", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private","stabs":{"name":"NotApproved"}}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given required fields are missing", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken, `{}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the build references a keyboard that doesn't exist", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"does-not-exist-`+uuid.NewString()+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the build references a switch that doesn't exist", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"switches":[{"switch":"does-not-exist-`+uuid.NewString()+`","count":1}]}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the build references a keycap set that doesn't exist", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"keycap_kits":[{"keycap_set":"does-not-exist-`+uuid.NewString()+`","kit":"kit1"}]}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the build references a keycap set that exists but not the kit", func() {
			var keycapSetID string

			BeforeEach(func(ctx SpecContext) {
				keycapSetID = "build-fixture-keycap-set-" + uuid.NewString()
				Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"keycap_kits":[{"keycap_set":"`+keycapSetID+`","kit":"does-not-exist"}]}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 with a problem+json body", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})

		Context("given the plate and PCB are ones the keyboard has", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"plate":"`+db.SeededPlateID+`","pcb":"`+db.SeededPCBID+`"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 201 with the plate and PCB resolved and the plate's price in total_cost", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))

					var got struct {
						ID    string `json:"id"`
						Plate *struct {
							ID       string `json:"id"`
							Material string `json:"material"`
						} `json:"plate"`
						PCB *struct {
							ID       string  `json:"id"`
							Firmware *string `json:"firmware"`
						} `json:"pcb"`
						TotalCost *float64 `json:"total_cost"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					createdID = got.ID
					Expect(got.Plate).NotTo(BeNil())
					Expect(got.Plate.ID).To(Equal(db.SeededPlateID))
					Expect(got.Plate.Material).To(Equal("FR4"))
					Expect(got.PCB).NotTo(BeNil())
					Expect(got.PCB.ID).To(Equal(db.SeededPCBID))
					Expect(*got.PCB.Firmware).To(Equal("QMK/VIA"))
					Expect(got.TotalCost).NotTo(BeNil())
					Expect(*got.TotalCost).To(Equal(369.99))
				})
			})
		})

		Context("given the plate isn't one the keyboard offers", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private","plate":"Polycarbonate"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 naming the plate field", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

					var got struct {
						InvalidParams []struct {
							Name string `json:"name"`
						} `json:"invalid_params"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.InvalidParams).To(ConsistOf(HaveField("Name", "plate")))
				})
			})
		})

		Context("given a durometer with a mount type that doesn't support one", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"case_mount_type":{"type":"Top Mount","durometer":"`+approvedDurometer+`"}}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400 naming the durometer field", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

					var got struct {
						InvalidParams []struct {
							Name string `json:"name"`
						} `json:"invalid_params"`
					}
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got.InvalidParams).To(ConsistOf(HaveField("Name", "case_mount_type.durometer")))
				})
			})
		})

		Context("given the build references a switch owned by another user", func() {
			var (
				otherOwnerID string
				switchID     string
			)

			BeforeEach(func(ctx SpecContext) {
				var err error
				_, otherOwnerID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				switchID = "build-fixture-switch-" + uuid.NewString()
				Expect(db.SeedSwitch(ctx, otherOwnerID, switchID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteSwitch(ctx, otherOwnerID, switchID)).To(Succeed())
			})

			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, ownerToken,
						`{"keyboard":"`+keyboardID+`","visibility":"private",`+
							`"switches":[{"switch":"`+switchID+`","count":1}]}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 400, treating another user's switch the same as one that doesn't exist", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
				})
			})
		})
	})

	Context("given the caller is anonymous", func() {
		Context("given an otherwise-valid body", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, "",
						`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
		})

		// Auth middleware runs before OpenAPI body validation, so a bad
		// token must win over a bad body.
		Context("given a body missing required fields", func() {
			When("creating a build", func() {
				BeforeEach(func(ctx SpecContext) {
					var err error
					resp, err = client.Create(ctx, ownerID, "", `{}`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns 401, not 400", func() {
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
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

		When("creating a build in the owner's collection", func() {
			BeforeEach(func(ctx SpecContext) {
				var err error
				resp, err = client.Create(ctx, ownerID, token,
					`{"keyboard":"`+keyboardID+`","visibility":"private"}`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns 404, not 403, to avoid revealing whose collection this is", func() {
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})
	})
})
