package keyboards_test

import (
	"encoding/json"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/kbdb/test/functional/support/api"
	"github.com/roguekeyboards/kbdb/test/functional/support/db"
)

var _ = Describe("Deleting a keyboard that is still referenced by a build, over MCP", func() {
	var (
		client       *api.MCPClient
		ownerID      string
		keyboardID   string
		buildID      string
		keyboardGone bool
		buildGone    bool
	)

	BeforeEach(func(ctx SpecContext) {
		keyboardGone = false
		buildGone = false
		keyboardID = "mcp-cascade-keyboard-" + uuid.NewString()
		buildID = "mcp-cascade-build-" + uuid.NewString()

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)

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
		When("the delete_keyboard tool is called", func() {
			It("fails and lists the blocking build id, leaving both in place", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_keyboard", map[string]any{
					"keyboard_id": keyboardID,
				})
				Expect(err).NotTo(HaveOccurred())

				By("returning a tool error")
				Expect(result.IsError).To(BeTrue())

				By("the keyboard still existing")
				getKb, getErr := client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getKb.IsError).To(BeFalse())

				By("the build still existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeFalse())
			})
		})
	})

	Context("given on_delete is cascade", func() {
		When("the delete_keyboard tool is called", func() {
			It("deletes the keyboard and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_keyboard", map[string]any{
					"keyboard_id": keyboardID,
					"on_delete":   "cascade",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())
				keyboardGone = true
				buildGone = true

				By("the tool output listing the deleted build id")
				raw, marshalErr := json.Marshal(result.StructuredContent)
				Expect(marshalErr).NotTo(HaveOccurred())

				var out struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.Unmarshal(raw, &out)).To(Succeed())
				Expect(out.DeletedBuildIDs).To(ContainElement(buildID))

				By("the keyboard no longer existing")
				getKb, getErr := client.CallTool(ctx, "get_keyboard", map[string]any{"keyboard_id": keyboardID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getKb.IsError).To(BeTrue())

				By("the build no longer existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeTrue())
			})
		})
	})
})
