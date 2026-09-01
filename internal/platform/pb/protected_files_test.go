package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// No file field ships in this phase. The assertion does, because phase 002 adds
// one and a file served to a stranger is exactly the failure Principle VII
// exists to prevent — and because the failure is silent: an unprotected file is
// not an error anywhere, it is a working download for anyone holding the URL.
func TestAnUnprotectedFileFieldRefusesTheBoot(t *testing.T) {
	t.Parallel()

	t.Run("PocketBase's default is the unsafe one", func(t *testing.T) {
		t.Parallel()

		// core/field_file.go:128-134, verbatim: "Note that by default all files
		// are publicly accessible." The zero value is the trap, which is why
		// the assertion is a boot gate and not a code review item.
		assert.False(t, (&core.FileField{Name: "scan"}).Protected)
	})

	t.Run("Protected false is refused", func(t *testing.T) {
		t.Parallel()

		c := core.NewBaseCollection("attachments")
		c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1, Protected: false})

		err := pb.AssertCollections([]*core.Collection{c})

		require.Error(t, err)
		assert.ErrorIs(t, err, pb.ErrFileFieldUnprotected)
		assert.Contains(t, err.Error(), "attachments")
		assert.Contains(t, err.Error(), "scan")
	})

	t.Run("Protected true is accepted", func(t *testing.T) {
		t.Parallel()

		c := core.NewBaseCollection("attachments")
		c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1, Protected: true})

		assert.NoError(t, pb.AssertCollections([]*core.Collection{c}))
	})

	t.Run("every unprotected field is named, not just the first", func(t *testing.T) {
		t.Parallel()

		c := core.NewBaseCollection("attachments")
		c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1})
		c.Fields.Add(&core.FileField{Name: "report", MaxSelect: 1})

		err := pb.AssertCollections([]*core.Collection{c})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "scan")
		assert.Contains(t, err.Error(), "report")
	})

	t.Run("a system collection gets no exemption", func(t *testing.T) {
		t.Parallel()

		// Unlike the API-rule check, this one is not scoped to non-system
		// collections: "any FileField" means any. PocketBase ships none, so the
		// rule costs nothing today and closes the door on an upgrade that adds
		// one.
		c := core.NewBaseCollection("_probe")
		c.System = true
		c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1})

		assert.ErrorIs(t, pb.AssertCollections([]*core.Collection{c}), pb.ErrFileFieldUnprotected)
	})
}
