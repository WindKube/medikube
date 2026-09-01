package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FR-037's default, named. The number itself is asserted among the other
// defaults in config_test.go; what is asserted here is the two things that
// number has to be and that a value check cannot see: that 730 is two years,
// and that it is a setting an operator reads and changes rather than a constant
// the purge job carries.
const (
	retentionKey     = "MEDIKUBE_RETENTION_AUDIT_DAYS"
	retentionDefault = 730
)

// purgeJob is the job the horizon must NOT be buried in. It is named as a path
// because that is the failure: a default that lives in the code that acts on it
// is one an operator cannot find, cannot change, and cannot be told about.
const purgeJob = "internal/service/audit/retention.go"

func TestTheAuditRetentionDefaultIsTwoYears(t *testing.T) {
	t.Parallel()

	cfg, err := loadFrom(minimalEnv())
	require.NoError(t, err)

	// Loaded and not read off the struct tag: the default an operator gets is
	// the one the parser applies, and a tag nothing parses is documentation.
	require.Equal(t, retentionDefault, cfg.Retention.AuditDays)

	assert.Equal(t, 2*365, cfg.Retention.AuditDays, "the default is two years of 365 days")

	// And two years on a calendar, which is the thing 730 is standing in for.
	// A window of two years is 730 days or 731 depending on whether it spans a
	// 29th of February, so the assertion is "within a leap day of two calendar
	// years" — wide enough to be true of every start date and narrow enough to
	// refuse two months, two hundred days or four years.
	for _, start := range []time.Time{
		time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2027, time.March, 1, 0, 0, 0, 0, time.UTC),   // spans no 29th
		time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), // spans 2024's
	} {
		t.Run(start.Format(time.DateOnly), func(t *testing.T) {
			t.Parallel()

			twoYears := start.Sub(start.AddDate(-2, 0, 0)).Hours() / 24

			assert.InDelta(t, twoYears, float64(cfg.Retention.AuditDays), 1,
				"%d days back from %s is not two years", cfg.Retention.AuditDays, start.Format(time.DateOnly))
		})
	}
}

// The other half of FR-037: a *documented setting*. The generic gate asserts
// that every variable appears in both documents (documented_test.go); it cannot
// see whether the value beside the name is the value the binary would use, and
// a document that advertises the wrong default is worse than one that omits it
// — an operator sizing a disk believes it.
func TestTheAuditRetentionDefaultIsTheOneTheDocumentsAdvertise(t *testing.T) {
	t.Parallel()

	cfg, err := loadFrom(minimalEnv())
	require.NoError(t, err)

	stated := strconv.Itoa(cfg.Retention.AuditDays)

	for _, doc := range []string{"README.md", quickstartPath} {
		t.Run(doc, func(t *testing.T) {
			t.Parallel()

			body := readDoc(t, doc)
			if doc == "README.md" {
				body = documentedEnvironment(t, body)
			}

			line := lineNaming(t, body, retentionKey)

			assert.Contains(t, line, stated,
				"%s documents %s without its actual default of %s: %q", doc, retentionKey, stated, line)
		})
	}
}

// An operator's value is what runs. A default nothing can override is a
// constant with an environment variable painted on it.
func TestAnOperatorsRetentionOverridesTheDefault(t *testing.T) {
	t.Parallel()

	environ := minimalEnv()
	environ[retentionKey] = "45"

	cfg, err := loadFrom(environ)
	require.NoError(t, err)

	assert.Equal(t, 45, cfg.Retention.AuditDays)
	assert.NotEqual(t, retentionDefault, cfg.Retention.AuditDays,
		"the configured horizon was ignored and the default used, so the setting is decorative")
}

// The horizon is not in the purge job. The job takes the number of days as a
// parameter and holds no opinion about it — which is what makes the setting
// reachable, and is the half of FR-037 that a value assertion cannot see.
func TestThePurgeJobCarriesNoRetentionDefaultOfItsOwn(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join(repoRoot(t), purgeJob))
	require.NoError(t, err, "%s is where the horizon must not be; if it moved, move this assertion with it", purgeJob)

	assert.NotContains(t, string(source), strconv.Itoa(retentionDefault),
		"%s names the retention default itself: the horizon belongs to %s, and a job that knows it "+
			"is a job that keeps running on it after an operator changes the setting", purgeJob, retentionKey)
}

// lineNaming returns the one line of body that names key, so a failure prints
// the documentation as written rather than the whole document.
func lineNaming(t *testing.T, body, key string) string {
	t.Helper()

	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, key) {
			return line
		}
	}

	require.Failf(t, "undocumented", "no line names %s", key)

	return ""
}
