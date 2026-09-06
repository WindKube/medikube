package architecture

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The If-Match the API accepts is the quoted entity-tag it sent (web.ETag);
// a $_etag signal seeded with the bare version makes every edit and delete
// a 422. Every place that seeds one goes through web.ETag.
func TestEveryEtagSignalIsSeededThroughWebETag(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "internal", "web", "views")

	var offences []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".templ") && !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_templ.go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}

		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		for number, line := range strings.Split(string(source), "\n") {
			if (strings.Contains(line, "_etag: ") || strings.Contains(line, "'If-Match': \"")) && !strings.Contains(line, "web.ETag(") {
				offences = append(offences, path+":"+strconv.Itoa(number+1)+": "+strings.TrimSpace(line))
			}
		}

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offences)
}
