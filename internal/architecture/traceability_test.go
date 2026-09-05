package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTraceabilityDocumentIsUpToDate is T324: traceability.md is generated,
// not hand-written, from spec.md and tasks.md — the same corpus the rest of
// this package's tests read. A requirement or success criterion that drifts
// out of sync with the generated join fails here rather than being caught,
// eventually, by a human re-deriving it.
//
// An unmapped FR, or an SC neither mapped nor marked [outcome metric] in
// spec.md, fails the phase per plan.md's exit criterion 1 — this test is
// that check.
func TestTraceabilityDocumentIsUpToDate(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	specPath := filepath.Join(root, "specs/001-walking-skeleton/spec.md")
	tasksPath := filepath.Join(root, "specs/001-walking-skeleton/tasks.md")
	docPath := filepath.Join(root, "specs/001-walking-skeleton/traceability.md")

	spec, err := os.ReadFile(specPath)
	require.NoError(t, err)

	tasks, err := os.ReadFile(tasksPath)
	require.NoError(t, err)

	frs, outcomeMetric := parseRequirementIDs(string(spec), "FR")
	scs, scOutcome := parseRequirementIDs(string(spec), "SC")
	for k, v := range scOutcome {
		outcomeMetric[k] = v
	}

	frTasks, scTasks := citingTasks(string(tasks), frs, scs)

	var unmapped []string

	for _, fr := range frs {
		if len(frTasks[fr]) == 0 {
			unmapped = append(unmapped, fr)
		}
	}

	assert.Emptyf(t, unmapped, "these functional requirements have no citing task: %v", unmapped)

	var unproven []string

	for _, sc := range scs {
		if len(scTasks[sc]) == 0 && !outcomeMetric[sc] {
			unproven = append(unproven, sc)
		}
	}

	assert.Emptyf(t, unproven,
		"these success criteria are neither mapped to a task nor marked [outcome metric] in spec.md: %v", unproven)

	scenarios := acceptanceScenarios(t)

	generated := renderTraceability(frs, frTasks, scs, scTasks, outcomeMetric, scenarios)

	committed, err := os.ReadFile(docPath)
	if err != nil {
		require.NoError(t, os.WriteFile(docPath, []byte(generated), 0o644))
		t.Fatalf("%s did not exist; it has been generated — re-run and commit it", docPath)
	}

	assert.Equal(t, string(committed), generated,
		"specs/001-walking-skeleton/traceability.md is stale — regenerate it from this test's output and commit the result")
}

// requirementLine matches "**FR-001**" or "**SC-001**" at the start of a
// bullet, capturing the optional "[outcome metric]" marker spec.md attaches
// to some success criteria.
var requirementLine = regexp.MustCompile(`(?m)^- \*\*(FR|SC)-(\d{3})\*\*(\s*\*\[outcome metric\]\*)?:`)

// parseRequirementIDs returns every id of the given kind ("FR" or "SC") from
// spec.md, in document order, plus which of them carry the [outcome metric]
// marker.
func parseRequirementIDs(spec, kind string) ([]string, map[string]bool) {
	var ids []string

	outcome := map[string]bool{}

	for _, m := range requirementLine.FindAllStringSubmatch(spec, -1) {
		if m[1] != kind {
			continue
		}

		id := fmt.Sprintf("%s-%s", m[1], m[2])
		ids = append(ids, id)
		outcome[id] = m[3] != ""
	}

	return ids, outcome
}

// taskLine matches a task list bullet: "- [ ] T123" or "- [x] T223a", capturing
// the task id including any letter suffix.
var taskLine = regexp.MustCompile(`^- \[[ xX]\] (T\d+[a-z]?)\b`)

// citingTasks walks tasks.md's task blocks — a bullet line and every
// following line indented as its continuation — and records, for every FR/SC
// id mentioned inside a block, the task id that block belongs to.
func citingTasks(doc string, frs, scs []string) (map[string][]string, map[string][]string) {
	frSet := toSet(frs)
	scSet := toSet(scs)

	frTasks := map[string][]string{}
	scTasks := map[string][]string{}

	idPattern := regexp.MustCompile(`(FR|SC)-\d{3}`)

	var currentID string

	var block strings.Builder

	flush := func() {
		if currentID == "" {
			return
		}

		for _, id := range idPattern.FindAllString(block.String(), -1) {
			switch {
			case frSet[id]:
				frTasks[id] = appendUnique(frTasks[id], currentID)
			case scSet[id]:
				scTasks[id] = appendUnique(scTasks[id], currentID)
			}
		}
	}

	for _, line := range strings.Split(doc, "\n") {
		if m := taskLine.FindStringSubmatch(line); m != nil {
			flush()

			currentID = m[1]
			block.Reset()
			block.WriteString(line)
			block.WriteByte('\n')

			continue
		}

		if currentID != "" && strings.HasPrefix(line, "  ") {
			block.WriteString(line)
			block.WriteByte('\n')

			continue
		}

		flush()
		currentID = ""
	}

	flush()

	for _, ids := range frTasks {
		sortTaskIDs(ids)
	}

	for _, ids := range scTasks {
		sortTaskIDs(ids)
	}

	return frTasks, scTasks
}

func toSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}

	return set
}

func appendUnique(list []string, id string) []string {
	for _, existing := range list {
		if existing == id {
			return list
		}
	}

	return append(list, id)
}

// sortTaskIDs orders "T18", "T133", "T223a" numerically on the digits and
// then by any trailing letter, so the rendered document reads in task order
// rather than lexical order ("T2" before "T18").
func sortTaskIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool {
		ni, si := splitTaskID(ids[i])
		nj, sj := splitTaskID(ids[j])
		if ni != nj {
			return ni < nj
		}

		return si < sj
	})
}

func splitTaskID(id string) (int, string) {
	digits := strings.TrimLeft(id, "T")

	i := 0
	for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
		i++
	}

	n, _ := strconv.Atoi(digits[:i])

	return n, digits[i:]
}

func renderTraceability(
	frs []string, frTasks map[string][]string,
	scs []string, scTasks map[string][]string, outcomeMetric map[string]bool,
	scenarios []string,
) string {
	var b strings.Builder

	b.WriteString("# Traceability\n\n")
	b.WriteString("Generated by `internal/architecture/traceability_test.go` from `spec.md` and\n")
	b.WriteString("`tasks.md`. Do not hand-edit — run the test and commit its output.\n\n")

	b.WriteString("## Functional requirements\n\n")
	b.WriteString("| Requirement | Tasks |\n|---|---|\n")

	for _, fr := range frs {
		ids := frTasks[fr]
		cell := "unproven"

		if len(ids) > 0 {
			cell = strings.Join(ids, ", ")
		}

		fmt.Fprintf(&b, "| %s | %s |\n", fr, cell)
	}

	b.WriteString("\n## Success criteria\n\n")
	b.WriteString("| Criterion | Tasks |\n|---|---|\n")

	for _, sc := range scs {
		ids := scTasks[sc]

		var cell string

		switch {
		case outcomeMetric[sc]:
			cell = "[outcome metric]"
		case len(ids) > 0:
			cell = strings.Join(ids, ", ")
		default:
			cell = "unproven"
		}

		fmt.Fprintf(&b, "| %s | %s |\n", sc, cell)
	}

	b.WriteString("\n## Acceptance scenarios\n\n")
	b.WriteString("| Scenario | Test |\n|---|---|\n")

	for _, id := range scenarios {
		test, proven := scenarioTests[id]
		if !proven {
			test = missingScenarios[id]
			if test == "" {
				test = "unproven"
			} else {
				test = "unproven — " + test
			}
		}

		fmt.Fprintf(&b, "| %s | %s |\n", id, test)
	}

	return b.String()
}
