package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T303, FR-070. A gate that exists but is not wired into CI is not a gate, so
// this reads .github/workflows/go.yaml itself rather than trusting that
// somebody remembered to add a job for it.
//
// Textual rather than a YAML decode: no YAML library is a direct dependency of
// this module (go.mod carries three only as transitive requirements of
// PocketBase and the OpenAPI tooling), and forbidden_deps_test.go already
// treats go.mod the same way rather than widen the dependency surface for a
// test. A workflow file's job names live at a fixed two-space indent under
// `jobs:`, which is regular enough to find without a parser.

const ciWorkflowPath = ".github/workflows/go.yaml"

var jobNamePattern = regexp.MustCompile(`(?m)^  ([A-Za-z0-9_-]+):\s*$`)

func TestCIWorkflowRunsEveryGate(t *testing.T) {
	t.Parallel()

	workflow := ciWorkflow(t)

	required := []string{
		"gen",
		"vet",
		"lint",
		"test",
		"phi-leak",
		"stream-liveness",
		"e2e",
		"openapi-diff",
	}

	jobs := jobNames(t, workflow)

	for _, job := range required {
		assert.Containsf(t, jobs, job, "%s declares no %q job", ciWorkflowPath, job)
	}
}

func TestCIWorkflowNeverPinsTheToolchainLocal(t *testing.T) {
	t.Parallel()

	workflow := ciWorkflow(t)

	// go.mod's toolchain directive resolves go1.27 for us; GOTOOLCHAIN=local
	// would turn that deliberate divergence into a misleading complaint about
	// go.mod instead (CLAUDE.md). Comment lines are excluded: the gen job's own
	// comment names GOTOOLCHAIN=local to explain why it is absent, which is not
	// the workflow setting it.
	for _, line := range strings.Split(workflow, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		assert.NotContainsf(t, line, "GOTOOLCHAIN=local", "an active line sets the toolchain locally: %q", line)
		assert.NotContainsf(t, line, "GOTOOLCHAIN: local", "an active line sets the toolchain locally: %q", line)
	}
}

// TestEveryTaskfileCIGateRunsInAJob asserts the other direction: a gate the
// Taskfile's own `ci` task lists (`task ci`, the thing a contributor runs
// before pushing) that CI itself never invokes would pass locally and never
// be checked at all.
func TestEveryTaskfileCIGateRunsInAJob(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yaml"))
	require.NoError(t, err)

	gates := ciTaskGates(t, string(taskfile))
	require.NotEmpty(t, gates, "parsed no gates out of the Taskfile's ci task — the parser is broken, not the task")

	workflow := ciWorkflow(t)

	for _, gate := range gates {
		assert.Containsf(t, workflow, "task "+gate,
			"the Taskfile's ci task runs %q, but %s never does", gate, ciWorkflowPath)
	}
}

func ciWorkflow(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ciWorkflowPath))
	require.NoError(t, err)

	return string(raw)
}

func jobNames(t *testing.T, workflow string) []string {
	t.Helper()

	jobsBlock := workflow
	if idx := strings.Index(workflow, "\njobs:\n"); idx >= 0 {
		jobsBlock = workflow[idx:]
	}

	matches := jobNamePattern.FindAllStringSubmatch(jobsBlock, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}

	require.NotEmpty(t, names, "found no job declarations in %s — the regex is broken, not the workflow", ciWorkflowPath)

	return names
}

// ciTaskGates reads the `ci:` task's own `cmds:` list out of the Taskfile —
// each entry of the shape `- task: <name>` — and returns the task names it
// runs. It is deliberately narrow to the one task this gate cares about: a
// textual scan of the whole file would also catch tasks unrelated to `ci`.
func ciTaskGates(t *testing.T, taskfile string) []string {
	t.Helper()

	idx := strings.Index(taskfile, "\n  ci:\n")
	require.GreaterOrEqual(t, idx, 0, "Taskfile.yaml declares no top-level ci: task")

	block := taskfile[idx+len("\n  ci:\n"):]

	// The block ends at the next task declared at the same two-space indent.
	if next := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:\s*$`).FindStringIndex(block); next != nil {
		block = block[:next[0]]
	}

	matches := regexp.MustCompile(`(?m)^\s*-\s*task:\s*(\S+)\s*$`).FindAllStringSubmatch(block, -1)
	gates := make([]string, 0, len(matches))
	for _, match := range matches {
		gates = append(gates, match[1])
	}

	return gates
}
