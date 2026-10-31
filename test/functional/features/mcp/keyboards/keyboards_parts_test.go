package keyboards_test

import (
	"encoding/json"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Managing a keyboard's plates and PCBs over MCP", func() {
	var (
		client     *api.MCPClient
		result     *sdkmcp.CallToolResult
		err        error
		ownerID    string
		keyboardID string
	)

	// readKeyboard calls get_keyboard and decodes it.
	readKeyboard := func(ctx SpecContext) getOutput {
		got, getErr := client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
		Expect(getErr).NotTo(HaveOccurred())
		Expect(got.IsError).To(BeFalse())
		return decodeGetOutput(got)
	}

	BeforeEach(func(ctx SpecContext) {
		result = nil
		err = nil
		keyboardID = "functional-test-keyboard-parts-" + uuid.NewString()

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)
		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given the caller owns a keyboard with one plate and one PCB", func() {
		Context("given a PCB with its own purchase", func() {
			When("the create_keyboard_pcb tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_keyboard_pcb", map[string]any{
						"keyboard_id": keyboardID,
						"firmware":    "ZMK",
						"assembly":    "Hotswap",
						"purchase":    map[string]any{"price": 45, "order_date": "2026-03-01"},
					})
				})

				It("adds it after the existing PCB with its own id, and counts its price", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					raw, marshalErr := json.Marshal(result.StructuredContent)
					Expect(marshalErr).NotTo(HaveOccurred())
					var created struct {
						PCB struct {
							ID string `json:"id"`
						} `json:"pcb"`
					}
					Expect(json.Unmarshal(raw, &created)).To(Succeed())
					Expect(created.PCB.ID).NotTo(BeEmpty())

					out := readKeyboard(ctx)
					Expect(out.Keyboard.PCBs).To(HaveLen(2))
					Expect(out.Keyboard.PCBs[0].ID).To(Equal(db.SeededPCBID))
					Expect(out.Keyboard.PCBs[1].ID).To(Equal(created.PCB.ID))
					Expect(out.Keyboard.TotalCost).NotTo(BeNil())
					Expect(*out.Keyboard.TotalCost).To(Equal(414.99))
				})
			})
		})

		Context("given changed details for the existing plate", func() {
			When("the update_keyboard_plate tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "update_keyboard_plate", map[string]any{
						"keyboard_id": keyboardID,
						"plate_id":    db.SeededPlateID,
						"material":    "PC",
					})
				})

				It("keeps the plate's id with its new details", func(ctx SpecContext) {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := readKeyboard(ctx)
					Expect(out.Keyboard.Plates).To(HaveLen(1))
					Expect(out.Keyboard.Plates[0].ID).To(Equal(db.SeededPlateID))
					Expect(out.Keyboard.Plates[0].Material).To(Equal("PC"))
				})
			})
		})

		Context("given a build uses the plate", func() {
			var buildID string

			BeforeEach(func(ctx SpecContext) {
				buildID = "functional-test-keyboard-parts-build-" + uuid.NewString()
				Expect(db.SeedBuildWithParts(ctx, ownerID, buildID, keyboardID, "private")).To(Succeed())
			})

			AfterEach(func(ctx SpecContext) {
				Expect(db.DeleteBuild(ctx, ownerID, buildID, keyboardID)).To(Succeed())
			})

			Context("given on_delete is omitted (defaults to block)", func() {
				When("the delete_keyboard_plate tool is called", func() {
					BeforeEach(func(ctx SpecContext) {
						result, err = client.CallTool(ctx, "delete_keyboard_plate", map[string]any{
							"keyboard_id": keyboardID,
							"plate_id":    db.SeededPlateID,
						})
					})

					It("fails naming the build, and keeps the plate", func(ctx SpecContext) {
						Expect(err).NotTo(HaveOccurred())
						Expect(result.IsError).To(BeTrue())
						Expect(result.Content).NotTo(BeEmpty())
						text, ok := result.Content[0].(*sdkmcp.TextContent)
						Expect(ok).To(BeTrue())
						Expect(text.Text).To(ContainSubstring(buildID))

						Expect(readKeyboard(ctx).Keyboard.Plates).To(HaveLen(1))
					})
				})
			})
		})
	})
})
