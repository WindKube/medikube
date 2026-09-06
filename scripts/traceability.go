// Command traceability generates specs/003-clinical-records/traceability.md
// from spec.md and tasks.md — the mechanical join T217a asks for, not a
// hand-written document. Run via `go run scripts/traceability.go` (wired as
// `task traceability`).
//
// It emits one row per functional requirement (FR-001..FR-094) naming the
// task ids that cite it, one row per acceptance scenario naming the task ids
// that cite it, and one row per success criterion (SC-001..SC-018) naming
// its task or "[outcome metric]". It exits non-zero — and writes nothing —
// for an FR with no citing task, or an SC neither cited nor marked
// [outcome metric] in spec.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	specRelPath  = "specs/003-clinical-records/spec.md"
	tasksRelPath = "specs/003-clinical-records/tasks.md"
	docRelPath   = "specs/003-clinical-records/traceability.md"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "traceability:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	spec, err := os.ReadFile(filepath.Join(root, specRelPath)) //nolint:gosec // fixed, repo-relative constant paths, not user input
	if err != nil {
		return err
	}

	tasks, err := os.ReadFile(filepath.Join(root, tasksRelPath)) //nolint:gosec // same as above
	if err != nil {
		return err
	}

	frs, outcomeMetric := parseRequirementIDs(string(spec), "FR")
	scs, scOutcome := parseRequirementIDs(string(spec), "SC")

	for k, v := range scOutcome {
		outcomeMetric[k] = v
	}

	scenarios := parseAcceptanceScenarios(string(spec))

	frTasks, scTasks, scenarioTasks := citingTasks(string(tasks), frs, scs, scenarioIDs(scenarios))

	var unmapped []string

	for _, fr := range frs {
		if len(frTasks[fr]) == 0 {
			unmapped = append(unmapped, fr)
		}
	}

	var unproven []string

	for _, sc := range scs {
		if len(scTasks[sc]) == 0 && !outcomeMetric[sc] {
			unproven = append(unproven, sc)
		}
	}

	// The document is written either way — it is the record of exactly this
	// state, gaps included — but a genuine gap still fails the run: that is
	// what "fails the phase" (T217a, cross-artifact finding M7) means in
	// practice, and it is why `task traceability` is a check, not only a
	// generator.
	generated := render(frs, frTasks, scs, scTasks, outcomeMetric, scenarios, scenarioTasks)

	if err := os.WriteFile(filepath.Join(root, docRelPath), []byte(generated), 0o600); err != nil {
		return err
	}

	if len(unmapped) > 0 || len(unproven) > 0 {
		if len(unmapped) > 0 {
			fmt.Fprintf(os.Stderr, "traceability: these functional requirements have no citing task: %v\n", unmapped)
		}

		if len(unproven) > 0 {
			fmt.Fprintf(os.Stderr, "traceability: these success criteria are neither mapped to a task nor marked [outcome metric]: %v\n", unproven)
		}

		return fmt.Errorf("%d unmapped FR, %d unproven SC", len(unmapped), len(unproven))
	}

	return nil
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not resolve this file's own path")
	}

	return filepath.Join(filepath.Dir(file), ".."), nil
}

// requirementLine matches "**FR-001**" or "**SC-001**" at the start of a
// bullet, capturing the optional "[outcome metric]" marker spec.md attaches
// to some success criteria.
var requirementLine = regexp.MustCompile(`(?m)^- \*\*(FR|SC)-(\d{3})\*\*(\s*\*\[outcome metric\]\*)?:`)

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

type scenario struct {
	id   string // "US3-2"
	text string
}

var (
	userStoryHeader = regexp.MustCompile(`^### User Story (\d+)`)
	scenariosHeader = regexp.MustCompile(`^\*\*Acceptance Scenarios\*\*:`)
	scenarioItem    = regexp.MustCompile(`^(\d+)\. (.*)`)
	sectionHeader   = regexp.MustCompile(`^#{2,3} `)
)

// parseAcceptanceScenarios walks spec.md's "### User Story N" sections and,
// within each one's "**Acceptance Scenarios**:" block, numbers every list
// item "USN-<item number>" — the same id spelling tasks.md's own USx-y
// annotations already use, so the join against tasks.md is exact rather than
// approximate.
func parseAcceptanceScenarios(spec string) []scenario {
	var scenarios []scenario

	story := ""
	inScenarios := false

	for _, line := range strings.Split(spec, "\n") {
		if m := userStoryHeader.FindStringSubmatch(line); m != nil {
			story = m[1]
			inScenarios = false

			continue
		}

		if scenariosHeader.MatchString(line) {
			inScenarios = story != ""

			continue
		}

		if !inScenarios {
			continue
		}

		if m := scenarioItem.FindStringSubmatch(line); m != nil {
			scenarios = append(scenarios, scenario{
				id:   fmt.Sprintf("US%s-%s", story, m[1]),
				text: m[2],
			})

			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		if sectionHeader.MatchString(line) || line == "---" {
			inScenarios = false
		}
	}

	return scenarios
}

func scenarioIDs(scenarios []scenario) []string {
	ids := make([]string, 0, len(scenarios))
	for _, s := range scenarios {
		ids = append(ids, s.id)
	}

	return ids
}

// taskLine matches a task list bullet: "- [ ] T123" or "- [x] T223a", capturing
// the task id including any letter suffix.
var taskLine = regexp.MustCompile(`^- \[[ xX]\] (T\d+[a-z]?)\b`)

// idPattern recognises FR-xxx, SC-xxx and USn-m citations inside a task's own
// text.
var idPattern = regexp.MustCompile(`(FR|SC)-\d{3}|US\d+-\d+`)

// citingTasks walks tasks.md's task blocks — a bullet line and every
// following line indented as its continuation — and records, for every
// FR/SC/scenario id mentioned inside a block, the task id that block belongs
// to.
func citingTasks(doc string, frs, scs, scenarioIDs []string) (map[string][]string, map[string][]string, map[string][]string) {
	frSet := toSet(frs)
	scSet := toSet(scs)
	scenarioSet := toSet(scenarioIDs)

	frTasks := map[string][]string{}
	scTasks := map[string][]string{}
	scenarioTasks := map[string][]string{}

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
			case scenarioSet[id]:
				scenarioTasks[id] = appendUnique(scenarioTasks[id], currentID)
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

	for _, ids := range scenarioTasks {
		sortTaskIDs(ids)
	}

	return frTasks, scTasks, scenarioTasks
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

func render(
	frs []string, frTasks map[string][]string,
	scs []string, scTasks map[string][]string, outcomeMetric map[string]bool,
	scenarios []scenario, scenarioTasks map[string][]string,
) string {
	var b strings.Builder

	b.WriteString("# Traceability\n\n")
	b.WriteString("Generated by `scripts/traceability.go` (`task traceability`) from `spec.md` and\n")
	b.WriteString("`tasks.md`. Do not hand-edit — run the generator and commit its output.\n\n")

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
	b.WriteString("| Scenario | Tasks |\n|---|---|\n")

	for _, s := range scenarios {
		ids := scenarioTasks[s.id]
		cell := "unproven"

		if len(ids) > 0 {
			cell = strings.Join(ids, ", ")
		}

		fmt.Fprintf(&b, "| %s | %s |\n", s.id, cell)
	}

	return b.String()
}
