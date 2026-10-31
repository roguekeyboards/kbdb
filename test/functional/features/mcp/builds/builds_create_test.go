package builds_test

import (
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/roguekeyboards/mykeebs-api/test/functional/support"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/api"
	"github.com/roguekeyboards/mykeebs-api/test/functional/support/db"
)

var _ = Describe("Creating a build over MCP", func() {
	var (
		client     *api.MCPClient
		result     *sdkmcp.CallToolResult
		err        error
		ownerID    string
		keyboardID string
		createdID  string
	)

	BeforeEach(func() {
		result = nil
		err = nil
		createdID = ""
	})

	captureCreatedID := func(r *sdkmcp.CallToolResult) {
		if r == nil || r.IsError {
			return
		}
		createdID = decodeBuildOutput(r).Build.ID
	}

	AfterEach(func(ctx SpecContext) {
		if createdID != "" {
			Expect(db.DeleteBuild(ctx, ownerID, createdID, keyboardID)).To(Succeed())
		}
		if keyboardID != "" {
			Expect(db.DeleteKeyboard(ctx, ownerID, keyboardID)).To(Succeed())
		}
	})

	Context("given a valid bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client, ownerID = api.NewAuthenticatedMCPClient(ctx)

			// A build references a keyboard, so seed one directly rather
			// than through the API - keeps this spec focused on the
			// create_build tool itself.
			keyboardID = "build-fixture-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		})

		Context("given valid arguments", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "private",
					})
					captureCreatedID(result)
				})

				It("creates the build in the caller's own collection", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())

					out := decodeBuildOutput(result)
					Expect(out.Build.ID).NotTo(BeEmpty(), "the server assigns the id")
					Expect(out.Build.Keyboard).To(Equal(keyboardID))
					Expect(out.Build.Visibility).To(Equal("private"))
					Expect(out.Build.HasImages).To(BeFalse())
				})
			})
		})

		Context("given every open-vocabulary field has an approved lookup value", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "private",
						"stabs": map[string]any{
							"name":       approvedStabilizer,
							"mount_type": approvedStabilizerMount,
						},
						"case_mount_type": map[string]any{
							"type":      approvedCaseMountType,
							"durometer": approvedDurometer,
						},
					})
					captureCreatedID(result)
				})

				It("creates the build", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeFalse())
					Expect(decodeBuildOutput(result).Build.ID).NotTo(BeEmpty())
				})
			})
		})

		Context("given an open-vocabulary field has an unapproved value", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "private",
						"stabs":      map[string]any{"name": "NotApproved"},
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given a required field is blank", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   "",
						"visibility": "private",
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given an invalid visibility", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "everyone",
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the build references a keyboard that doesn't exist", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   "does-not-exist-" + uuid.NewString(),
						"visibility": "private",
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})

		Context("given the build references a switch that doesn't exist", func() {
			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "private",
						"switches": []map[string]any{
							{"switch": "does-not-exist-" + uuid.NewString(), "count": 1},
						},
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
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

			When("the create_build tool is called", func() {
				BeforeEach(func(ctx SpecContext) {
					result, err = client.CallTool(ctx, "create_build", map[string]any{
						"keyboard":   keyboardID,
						"visibility": "private",
						"keycap_kits": []map[string]any{
							{"keycap_set": keycapSetID, "kit": "does-not-exist"},
						},
					})
					captureCreatedID(result)
				})

				It("returns an MCP tool error result", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsError).To(BeTrue())
				})
			})
		})
	})

	Context("given no bearer token", func() {
		BeforeEach(func(ctx SpecContext) {
			client = api.NewMCPClient(support.BaseURL()+"/mcp", "")

			_, ownerID, err = api.NewAuthIdentity(ctx)
			Expect(err).NotTo(HaveOccurred())

			keyboardID = "build-fixture-keyboard-" + uuid.NewString()
			Expect(db.SeedKeyboard(ctx, ownerID, keyboardID, "private")).To(Succeed())
		})

		When("the create_build tool is called", func() {
			BeforeEach(func(ctx SpecContext) {
				result, err = client.CallTool(ctx, "create_build", map[string]any{
					"keyboard":   keyboardID,
					"visibility": "private",
				})
			})

			It("rejects the call with a real HTTP 401", func() {
				Expect(result).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
