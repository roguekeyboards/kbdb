package keycapsets_test

import (
	"encoding/json"

	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Deleting a keycap set with a kit that is still referenced by a build, over MCP", func() {
	var (
		client     *api.MCPClient
		ownerID    string
		setID      string
		kitID      string
		buildID    string
		keyboardID string
		setGone    bool
		buildGone  bool
	)

	BeforeEach(func(ctx SpecContext) {
		setGone = false
		buildGone = false
		setID = "mcp-cascade-keycap-set-" + uuid.NewString()
		kitID = "mcp-cascade-kit-" + uuid.NewString()
		buildID = "mcp-cascade-build-" + uuid.NewString()
		keyboardID = "mcp-cascade-keyboard-" + uuid.NewString()

		client, ownerID = api.NewAuthenticatedMCPClient(ctx)

		Expect(db.SeedKeycapSetWithKit(ctx, ownerID, setID, kitID, "private")).To(Succeed())
		Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
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
		When("the delete_keycap_set tool is called", func() {
			It("fails and lists the blocking build id, leaving both in place", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_keycap_set", map[string]any{
					"keycap_set_id": setID,
				})
				Expect(err).NotTo(HaveOccurred())

				By("returning a tool error")
				Expect(result.IsError).To(BeTrue())

				By("the set still existing")
				getSet, getErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": setID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getSet.IsError).To(BeFalse())

				By("the build still existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeFalse())
			})
		})
	})

	Context("given on_delete is cascade", func() {
		When("the delete_keycap_set tool is called", func() {
			It("deletes the set and the referencing build, returning the deleted build id", func(ctx SpecContext) {
				result, err := client.CallTool(ctx, "delete_keycap_set", map[string]any{
					"keycap_set_id": setID,
					"on_delete":     "cascade",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())
				setGone = true
				buildGone = true

				By("the tool output listing the deleted build id")
				raw, marshalErr := json.Marshal(result.StructuredContent)
				Expect(marshalErr).NotTo(HaveOccurred())

				var out struct {
					DeletedBuildIDs []string `json:"deleted_build_ids"`
				}
				Expect(json.Unmarshal(raw, &out)).To(Succeed())
				Expect(out.DeletedBuildIDs).To(ContainElement(buildID))

				By("the set no longer existing")
				getSet, getErr := client.CallTool(ctx, "get_keycap_set", map[string]any{"keycap_set_id": setID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getSet.IsError).To(BeTrue())

				By("the build no longer existing")
				getBuild, getErr := client.CallTool(ctx, "get_build", map[string]any{"build_id": buildID})
				Expect(getErr).NotTo(HaveOccurred())
				Expect(getBuild.IsError).To(BeTrue())
			})
		})
	})
})
