package phileak

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cross-artifact finding M6. There is one PHI-leak harness in this repository
// and this is it.
//
// The failure mode M6 describes is not a duplicate file; it is three partial
// gates. A logcapture.go beside the logger asserts over the log stream, an
// internal/obs/phi_leak_test.go asserts over the span recorder, and each of them
// looks, in review, exactly like the assertion — so the metric label nobody
// captured never gets captured and the suite stays green. Extending the
// exercise in this package is the only way that stays a single question.
func TestNoSecondPHILeakHarnessExists(t *testing.T) {
	t.Parallel()

	// The marker: a second span capture has to import this, and nothing else
	// has a reason to. It is the sink most easily captured by accident, in the
	// package that already owns the tracer.
	const spanRecorder = `"go.opentelemetry.io/otel/sdk/trace/tracetest"`

	// The two filenames the finding names, so that reintroducing either by
	// copying from another project fails here rather than in review.
	forbiddenNames := []string{"logcapture.go", "phi_leak_test.go", "phileak_capture.go"}

	root := repoRoot(t)
	mine := filepath.Join(root, "internal", "testsupport", "phileak")

	var (
		offences []string
		scanned  int
	)

	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			name := entry.Name()
			if name != "." && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "pb_data") {
				return fs.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		relative, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)

		for _, forbidden := range forbiddenNames {
			if entry.Name() == forbidden {
				offences = append(offences, relative+" is a second PHI-leak capture")
			}
		}

		if strings.HasPrefix(path, mine) {
			return nil
		}

		scanned++

		source, readErr := os.ReadFile(path) //nolint:gosec // a repository walk over its own files
		require.NoError(t, readErr)

		if strings.Contains(string(source), spanRecorder) {
			offences = append(offences, relative+" captures spans outside the one harness")
		}

		return nil
	}))

	require.Greater(t, scanned, 50, "the walk found almost nothing, so it proves nothing")
	assert.Empty(t, offences,
		"extend internal/testsupport/phileak's exercise instead — a second harness is a partial gate that reads like a complete one")
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	root := filepath.Join(filepath.Dir(file), "..", "..", "..")

	_, err := os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "%s is not the repository root", root)

	return root
}
