package profiles_test

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/rogueserenity/kbdb/test/functional/support/api"
	"github.com/rogueserenity/kbdb/test/functional/support/db"
)

// accountItems is one of each item a profile delete must remove: a
// keyboard and a switch with images, a keycap set with a kit, and a build
// referencing the keyboard.
type accountItems struct {
	keyboardID, switchID, keycapSetID, buildID string
}

func seedAccountItems(ctx context.Context, ownerID string) accountItems {
	ids := accountItems{
		keyboardID:  uuid.NewString(),
		switchID:    uuid.NewString(),
		keycapSetID: uuid.NewString(),
		buildID:     uuid.NewString(),
	}
	Expect(db.SeedKeyboardWithImage(ctx, ownerID, ids.keyboardID, uuid.NewString(), "private")).To(Succeed())
	Expect(db.SeedSwitchWithImage(ctx, ownerID, ids.switchID, "public")).To(Succeed())
	Expect(db.SeedKeycapSetWithKit(ctx, ownerID, ids.keycapSetID, uuid.NewString(), "private")).To(Succeed())
	Expect(db.SeedBuild(ctx, ownerID, ids.buildID, ids.keyboardID, "public")).To(Succeed())
	return ids
}

// cleanUp removes the items best-effort, in case a spec failed before the
// delete under test landed.
func (ids accountItems) cleanUp(ctx context.Context, ownerID string) {
	_ = db.DeleteBuild(ctx, ownerID, ids.buildID, ids.keyboardID)
	_ = db.DeleteKeyboard(ctx, ownerID, ids.keyboardID)
	_ = db.DeleteSwitch(ctx, ownerID, ids.switchID)
	_ = db.DeleteKeycapSet(ctx, ownerID, ids.keycapSetID)
}

func (ids accountItems) expectAllGone(ctx context.Context, ownerID, token string) {
	keyboard, err := api.NewKeyboardsClient().Get(ctx, ownerID, ids.keyboardID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = keyboard.Body.Close() }()
	Expect(keyboard.StatusCode).To(Equal(http.StatusNotFound), "keyboard")

	sw, err := api.NewSwitchesClient().Get(ctx, ownerID, ids.switchID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = sw.Body.Close() }()
	Expect(sw.StatusCode).To(Equal(http.StatusNotFound), "switch")

	set, err := api.NewKeycapSetsClient().Get(ctx, ownerID, ids.keycapSetID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = set.Body.Close() }()
	Expect(set.StatusCode).To(Equal(http.StatusNotFound), "keycap set")

	build, err := api.NewBuildsClient().Get(ctx, ownerID, ids.buildID, token)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = build.Body.Close() }()
	Expect(build.StatusCode).To(Equal(http.StatusNotFound), "build")
}
