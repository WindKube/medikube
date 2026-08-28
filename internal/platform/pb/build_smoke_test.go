package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/stretchr/testify/require"
)

// If this test fails to COMPILE, the Go toolchain is too old — check your Go
// version before looking anywhere else.
//
// PocketBase v0.40.1 imports the Go 1.27 stdlib package encoding/json/v2 in 67
// non-test files, so on Go 1.26.5 this does not fail an assertion, it fails at
// compile time (VERIFIED-SOURCE-FACTS FACT 0). That is the whole point of the
// test and of running it first: every other package in this repository is built
// on top of PocketBase, and a toolchain that cannot build PocketBase produces
// error messages that point everywhere except at the real cause.
func TestPocketBaseBuildsAndConstructs(t *testing.T) {
	t.Parallel()

	// NewWithConfig rather than New: New reads os.Args, which under `go test`
	// carries the test binary's own flags. DefaultDataDir keeps the constructor
	// from touching the working tree.
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})

	require.NotNil(t, app, "pocketbase.NewWithConfig returned nil")
}
