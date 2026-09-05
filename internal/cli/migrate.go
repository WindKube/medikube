package cli

import (
	"errors"
	"slices"
)

// ErrMigrateDownRefused is FR-059's refusal: the down of migration 3 drops
// audit_events.
var ErrMigrateDownRefused = errors.New(
	"cli: medikube migrate down is refused in production without --force, because it drops audit_events")

// GuardMigrateDown refuses `down` in production unless --force is present,
// stripping --force before handing the rest back — PocketBase's migrate
// command defines no such flag. args is everything after "migrate" itself.
func GuardMigrateDown(args []string, env string) ([]string, error) {
	if len(args) == 0 || args[0] != "down" {
		return args, nil
	}

	forced := slices.Contains(args, "--force") || slices.Contains(args, "-force")

	if env == "production" && !forced {
		return nil, ErrMigrateDownRefused
	}

	if !forced {
		return args, nil
	}

	stripped := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--force" || arg == "-force" {
			continue
		}

		stripped = append(stripped, arg)
	}

	return stripped, nil
}
