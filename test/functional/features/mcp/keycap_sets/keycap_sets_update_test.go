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

var _ = Describe("Updating a keycap set over MCP", func() {
	var (
		client      *api.MCPClient
		result      *sdkmcp.CallToolResult
		err         error
		ownerID     string
		keycapSetID string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		keycapSetID = "functional-test-keycap-set-" + uuid.NewString()
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		})

		Context("given the caller owns the keycap set", func() {
			BeforeEach(func(ctx SpecContext) {
				Expect(db.SeedKeycapSet(ctx, ownerID, keycapSetID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_set tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"brand":         "GMK",
						"name":          "Laser V2",
						"visibility":    "private",
					})
				})

				It("replaces the keycap set's fields, persisted", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeKeycapSetOutput(result)
					Expect(out.KeycapSet.ID).To(Equal(keycapSetID))
					Expect(out.KeycapSet.Name).To(Equal("Laser V2"))

					By("actually persisting the new name, not a no-op")
					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(decodeKeycapSetOutput(check).KeycapSet.Name).To(Equal("Laser V2"))
				})
			})

			Context("given a request changing visibility to public", func() {
				When("the update_keycap_set tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
							"keycap_set_id": keycapSetID,
							"brand":         "GMK",
							"name":          "Laser",
							"visibility":    "public",
						})
					})

					It("makes the keycap set visible to another authenticated user", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeFalse())

						otherToken, _, tokenErr := api.NewAuthIdentity(ctx)
						Expect(tokenErr).NotTo(HaveOccurred())
						otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

						check, checkErr := otherClient.CallTool(ctx, "get_keycap_set", map[string]any{
							"keycap_set_id": keycapSetID,
							"user_id":       ownerID,
						})
						Expect(checkErr).NotTo(HaveOccurred())
						Expect(check.IsError).To(BeFalse())
						Expect(decodeKeycapSetOutput(check).KeycapSet.ID).To(Equal(keycapSetID))
					})
				})
			})

			Context("given an open-vocabulary field has an unapproved value", func() {
				When("the update_keycap_set tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
							"keycap_set_id": keycapSetID,
							"brand":         "GMK",
							"name":          "Laser",
							"visibility":    "private",
							"profile":       "NotApproved",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})

			Context("given a required field is present but blank", func() {
				When("the update_keycap_set tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
							"keycap_set_id": keycapSetID,
							"brand":         "",
							"name":          "Laser",
							"visibility":    "private",
						})
					})

					It("returns an MCP tool error result", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
					})
				})
			})
		})

		Context("given the keycap set has an optional field set", func() {
			BeforeEach(func(ctx SpecContext) {
				createResult, createErr := client.CallTool(ctx, "create_keycap_set", map[string]any{
					"brand":      "GMK",
					"name":       "Laser",
					"visibility": "private",
					"notes":      "group buy",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				keycapSetID = decodeKeycapSetOutput(createResult).KeycapSet.ID
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_set tool is called omitting that field", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"brand":         "GMK",
						"name":          "Laser",
						"visibility":    "private",
					})
				})

				It("clears the omitted field, since every field is replaced", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeKeycapSetOutput(result).KeycapSet.Notes).To(BeNil())
				})
			})
		})

		Context("given the keycap set has a kit", func() {
			BeforeEach(func(ctx SpecContext) {
				createResult, createErr := client.CallTool(ctx, "create_keycap_set", map[string]any{
					"brand":      "GMK",
					"name":       "Laser",
					"visibility": "private",
				})
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createResult.IsError).To(BeFalse())
				keycapSetID = decodeKeycapSetOutput(createResult).KeycapSet.ID

				kitResult, kitErr := client.CallTool(ctx, "create_keycap_kit", map[string]any{
					"keycap_set_id": keycapSetID,
					"name":          "Base",
				})
				Expect(kitErr).NotTo(HaveOccurred())
				Expect(kitResult.IsError).To(BeFalse())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, ownerID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_set tool is called changing only the set's own fields", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"brand":         "GMK",
						"name":          "Laser V2",
						"visibility":    "private",
					})
				})

				It("preserves the kit rather than wiping it", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					check, checkErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())

					kits := decodeKeycapSetOutput(check).KeycapSet.Kits
					Expect(kits).To(HaveLen(1))
					Expect(kits[0].Name).To(Equal("Base"))
				})
			})
		})

		Context("given the keycap set never existed", func() {
			When("the update_keycap_set tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"brand":         "GMK",
						"name":          "Laser",
						"visibility":    "private",
					})
				})

				It("returns an MCP tool error result, not idempotent like delete", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given another user owns the keycap set", func() {
			var otherID, otherToken string

			BeforeEach(func(ctx SpecContext) {
				otherToken, otherID, err = api.NewAuthIdentity(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(db.SeedKeycapSet(ctx, otherID, keycapSetID, "public")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteKeycapSet(ctx, otherID, keycapSetID)).To(Succeed())
			})

			When("the update_keycap_set tool is called with that id", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
						"keycap_set_id": keycapSetID,
						"brand":         "Hijacked",
						"name":          "Hijacked",
						"visibility":    "public",
					})
				})

				It("leaves the other user's keycap set untouched", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())

					otherClient := api.NewMCPClient(support.BaseURL()+"/mcp", otherToken)

					check, checkErr := otherClient.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": keycapSetID})
					Expect(checkErr).NotTo(HaveOccurred())
					Expect(check.IsError).To(BeFalse())
					Expect(decodeKeycapSetOutput(check).KeycapSet.Brand).To(Equal("GMK"))
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func() {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")
		})

		When("the update_keycap_set tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "update_keycap_set", map[string]any{
					"keycap_set_id": keycapSetID,
					"brand":         "GMK",
					"name":          "Laser",
					"visibility":    "private",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
