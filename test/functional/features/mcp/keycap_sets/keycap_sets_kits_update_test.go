package keycapsets_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support"
	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Updating a keycap kit over MCP", func() {
	var (
		client      *api.MCPClient
		result      *sdkmcp.CallToolResult
		err         error
		ownerID     string
		keycapSetID string
		kitID       string
	)

	BeforeEach(func() {
		result = nil
		err = nil
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns a keycap set with an existing kit", func() {
			BeforeEach(func(ctx SpecContext) {
				keycapSetID = "update-kit-set-" + uuid.NewString()
				Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())

				createResult, createErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
					"keycap_set_id": keycapSetID,
					"name":          "Base",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				kitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_kit tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"name":          "Base V2",
					})
				})

				It("updates the kit, embedded in the parent set", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapKitOutput(result)
					Expect(out.KeycapKit.KitID).To(Equal(kitID))
					Expect(out.KeycapKit.Name).To(Equal("Base V2"))

					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())

					kits := decodeKeycapSetOutput(check).KeycapSet.Kits
					Expect(kits).To(HaveLen(1))
					Expect(kits[0].Name).To(Equal("Base V2"))
				})
			})

			Context("given primary is true", func() {
				When("the update_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"name":          "Base V2",
							"primary":       true,
						})
					})

					It("makes the kit the set's primary_kit_id", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						primaryKitID := decodeKeycapSetOutput(check).KeycapSet.PrimaryKitID
						Expect(primaryKitID).NotTo(BeNil())
						Expect(*primaryKitID).To(Equal(kitID))
					})
				})
			})

			Context("given the kit is the set's current primary kit and primary is false", func() {
				BeforeEach(func(ctx SpecContext) {
					setPrimaryResult, setPrimaryErr := client.CallTool(ctx, "update_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"name":          "Base",
						"primary":       true,
					})
					Expect(setPrimaryErr).NotTo(HaveOccurred())
					Expect(setPrimaryResult.IsError).To(BeFalse())
				})

				When("the update_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"name":          "Base V2",
							"primary":       false,
						})
					})

					It("clears the set's primary_kit_id", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(decodeKeycapSetOutput(check).KeycapSet.PrimaryKitID).To(BeNil())
					})
				})
			})

			Context("given a required field is present but blank", func() {
				When("the update_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"name":          "",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the kit_id does not exist within the set", func() {
				When("the update_keycap_kit tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        "no-such-kit-" + uuid.NewString(),
							"name":          "Base V2",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given the set has a second, sibling kit", func() {
				var siblingKitID string

				BeforeEach(func(ctx SpecContext) {
					createResult, createErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"name":          "Extension",
					})
					Expect(createErr).NotTo(HaveOccurred())
					Expect(createResult.IsError).To(BeFalse())
					siblingKitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
				})

				When("the update_keycap_kit tool is called for the first kit", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
							"keycap_set_id": keycapSetID,
							"kit_id":        kitID,
							"name":          "Base V2",
						})
					})

					It("leaves the sibling kit unchanged", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
						Expect(checkErr).NotTo(HaveOccurred())

						kits := decodeKeycapSetOutput(check).KeycapSet.Kits
						Expect(kits).To(HaveLen(2))

						byID := map[string]string{}
						for _, k := range kits {
							byID[k.KitID] = k.Name
						}
						Expect(byID[kitID]).To(Equal("Base V2"))
						Expect(byID[siblingKitID]).To(Equal("Extension"))
					})
				})
			})
		})

		Context("given another user owns the keycap set", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				keycapSetID = "update-kit-set-" + uuid.NewString()
				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "public")).To(Succeed())

				otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)
				createResult, createErr := otherClient.CallTool(ctx, "create_keycap_kit", map[string]any{
					"keycap_set_id": keycapSetID,
					"name":          "Base",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				kitID = decodeKeycapKitOutput(createResult).KeycapKit.KitID
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_kit tool is called with that keycap_set_id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
						"keycap_set_id": keycapSetID,
						"kit_id":        kitID,
						"name":          "Hijacked",
					})
				})

				It("returns an MCP tool error result and leaves the kit untouched", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())

					kits := decodeKeycapSetOutput(check).KeycapSet.Kits
					Expect(kits).To(HaveLen(1))
					Expect(kits[0].Name).To(Equal("Base"))
				})
			})
		})

		Context("given the parent keycap set does not exist", func() {
			When("the update_keycap_kit tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
						"keycap_set_id": "no-such-keycap-set-" + uuid.NewString(),
						"kit_id":        "some-kit-id",
						"name":          "Base V2",
					})
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the update_keycap_kit tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_keycap_kit", map[string]any{
					"keycap_set_id": "irrelevant-" + uuid.NewString(),
					"kit_id":        "irrelevant",
					"name":          "Base V2",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
