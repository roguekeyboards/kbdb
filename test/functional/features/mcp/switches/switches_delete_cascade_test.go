package switches_test

import (
	"encoding/json"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a switch that is still referenced by a build, over MCP", func() {
	var (
		client     *api.MCPClient
		ownerID    string
		switchID   string
		buildID    string
		keyboardID string
		switchGone bool
		buildGone  bool
	)

	BeforeEach(func(ctx SpecContext) {
		switchGone = false
		buildGone = false
		switchID = "mcp-cascade-switch-" + uuid.NewString()
		buildID = "mcp-cascade-build-" + uuid.NewString()
		keyboardID = "mcp-cascade-keyboard-" + uuid.NewString()

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)

		Expect(db.SeedSwitch(ctx, ownerID, switchID, "private")).To(Succeed())
		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		Expect(db.SeedBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, switchID, "private")).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		if !buildGone {
			Expect(db.DeleteBuildWithSwitchAndKeyboard(ctx, ownerID, buildID, keyboardID, switchID)).To(Succeed())
		}
		if !switchGone {
			Expect(db.DeleteSwitch(ctx, ownerID, switchID)).To(Succeed())
		}
		Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
	})

	Context("given on_delete is omitted (defaults to block)", func() {
		When("the delete_switch tool is called", func() {
			It("fails and lists the blocking build id, leaving both in place", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_switch", map[string]any{
					"switch_id": switchID,
				})
				Expect(err).NotTo(HaveOccurred())

				By("returning a tool error")
				Expect(result.IsError).To(BeTrue())

				By("the switch still existing")
				getSw, getErr := client.CallTool(ctx, "get_switch", map[string]any{"switch_id": switchID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getSw.IsError).To(BeFalse())

				By("the build still existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeFalse())
			})
		})
	})

	Context("given on_delete is cascade", func() {
		When("the delete_switch tool is called", func() {
			It("deletes the switch and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_switch", map[string]any{
					"switch_id": switchID,
					"on_delete": "cascade",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())
				switchGone = true
				buildGone = true

				By("the tool output listing the deleted build id")
				raw, marshalErr := json.Marshal(result.StructuredContent)
				Expect(marshalErr).NotTo(HaveOccurred())

				var out struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.Unmarshal(raw, &out)).To(Succeed())
				Expect(out.DeletedBuildIDs).To(ContainElement(buildID))

				By("the switch no longer existing")
				getSw, getErr := client.CallTool(ctx, "get_switch", map[string]any{"switch_id": switchID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getSw.IsError).To(BeTrue())

				By("the build no longer existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeTrue())
			})
		})
	})
})
