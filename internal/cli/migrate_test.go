package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
)

// FR-059: `migrate down` is refused in production without --force, because
// the down of migration 3 drops audit_events.
func TestGuardMigrateDown(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args     []string
		env      string
		wantArgs []string
		wantErr  bool
	}{
		"down in production is refused": {
			args:    []string{"down"},
			env:     "production",
			wantErr: true,
		},
		"down --force in production strips the flag and proceeds": {
			args:     []string{"down", "--force"},
			env:      "production",
			wantArgs: []string{"down"},
		},
		"down --force before other arguments still strips only the flag": {
			args:     []string{"down", "--force", "2"},
			env:      "production",
			wantArgs: []string{"down", "2"},
		},
		"down outside production needs no --force": {
			args:     []string{"down"},
			env:      "development",
			wantArgs: []string{"down"},
		},
		"down --force outside production still strips the flag": {
			args:     []string{"down", "--force"},
			env:      "staging",
			wantArgs: []string{"down"},
		},
		"up is never guarded": {
			args:     []string{"up"},
			env:      "production",
			wantArgs: []string{"up"},
		},
		"no arguments at all is never guarded": {
			args:     []string{},
			env:      "production",
			wantArgs: []string{},
		},
		"history-sync is never guarded": {
			args:     []string{"history-sync"},
			env:      "production",
			wantArgs: []string{"history-sync"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := cli.GuardMigrateDown(tc.args, tc.env)

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, cli.ErrMigrateDownRefused)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantArgs, got)
		})
	}
}
