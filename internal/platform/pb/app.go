package pb

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/config"
)

// Options carries the seams the composition root fills in. They are separate
// from config.Config because they are functions, not values an operator sets.
type Options struct {
	// DBConnect is where database instrumentation attaches (research D-30).
	// Left nil, PocketBase uses core.DefaultDBConnect, so an uninstrumented
	// build is a valid build rather than a nil dereference at bootstrap.
	//
	// PocketBase calls it four times per boot — the data and auxiliary
	// databases, each concurrent and non-concurrent (core/base.go:1240, :1248,
	// :1302, :1310) — so anything with a per-connection cost pays it four
	// times.
	DBConnect core.DBConnectFunc
}

// New constructs the embedded PocketBase instance from MediKube's validated
// configuration. It does not bootstrap: connections, migrations and settings
// are all still closed, which is what lets the composition root decide their
// order.
//
// NewWithConfig rather than New, because DefaultDataDir and DBConnect must both
// be settled before bootstrap and pocketbase.New reads os.Args for them
// (research D-06).
func New(cfg config.Config, opts Options) *pocketbase.PocketBase {
	return pocketbase.NewWithConfig(pocketbase.Config{
		// FR-061: everything this instance keeps lives under one configured
		// directory. PocketBase's own fallback puts pb_data beside the
		// executable, which inside the image is a read-only layer, and that
		// failure surfaces long after boot.
		DefaultDataDir: cfg.DataDir,
		DefaultDev:     cfg.Dev,
		// The banner is PocketBase's own stdout write, and it would be the one
		// line in the process that is not JSON (Principle VI).
		HideStartBanner: true,
		DBConnect:       opts.DBConnect,
	})
}
