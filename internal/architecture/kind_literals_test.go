package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T046, the mechanical form of research D-05. A kind has three spellings —
// the enum value, the plural path segment and the collection — declared side by
// side in one table precisely because they will not always agree: `insurance`
// and `family-history` are not mechanical plurals of anything. Cross-artifact finding H1 is what happens when a second file
// spells one of them by hand — a route says one thing, a page says another, and
// nothing fails until somebody clicks.
//
// So the spelling exists once. Everywhere else calls Segment() or Collection().
const kindDeclaration = "internal/domain/kind/kind.go"

// Files that may hold the literal anyway, each because pinning the spelling is
// the whole point of the file. Anything not listed here calls the accessor.
var kindLiteralExempt = map[string]string{
	"internal/domain/kind/kind_test.go": "the golden table: the one place that pins each kind's plural spelling, which is finding H1's fix",
	"internal/domain/audit/enums_test.go": "the near-miss list: it asserts the plural is refused as an audit target kind, " +
		"which is the same drift caught from the other side",
	"internal/domain/audit/enums.go": "declares audit.TargetKind, its own vocabulary of the kind's enum spelling — " +
		"pinning that spelling is the whole point of the file, exactly as kind.go pins the other three",
	"internal/store/migrations/1756100300_audit_events.go": "the migration that writes audit_events.target_kind's " +
		"select values: the complete vocabulary is pinned here deliberately (data-model §5.4) and is asserted " +
		"against kind.Kinds() by audit_vocab_test.go, not derived from it",
	"internal/testsupport/phileak/exercise_test.go": "a false positive: the flagged literal is the English word " +
		"\"precondition\", which merely contains condition's collection name as a substring",
	"internal/architecture/enum_slices_test.go": "a false positive: two flagged identifiers in enumSlices name a " +
		"per-kind vocabulary slice whose own name happens to contain a kind's spelling as a substring",
	"internal/architecture/kind_literals_test.go": "this file's own exemption map necessarily spells every " +
		"exempted path and reason below as a literal, including the vitals ones",

	// Equipment and insurance are the first two kinds whose segment and
	// collection are an uncountable noun spelled identically to the kind's
	// own ordinary English name (every other kind's segment is a distinct
	// plural, e.g. medication/medications, so an error message or a log
	// field prefixed with the kind's name in prose never collides with its
	// own path spelling). Every file below is a false positive of that
	// shape: an error message, a zerolog field name or a struct member that
	// says "equipment" or "insurance" because that is what the thing is
	// called, not because it hardcodes a route or a collection.
	"internal/domain/clinical/equipment.go":              "false positive: equipmentIDField says what the log field is",
	"internal/domain/clinical/equipment_test.go":         "false positive, asserting the same field name",
	"internal/domain/clinical/insurance.go":              "false positive: insuranceIDField says what the log field is",
	"internal/domain/clinical/insurance_test.go":         "false positive, asserting the same field name",
	"internal/domain/clinical/insurancecoverage.go":      "false positive: coverage.Validate's field-error messages name insurance's own coverage/contact fields in prose",
	"internal/domain/clinical/insurancecoverage_test.go": "false positive, asserting the same messages",
	"internal/httproute/routes.go":                       "false positive: the two kinds' route summaries and landmark names say what the page is about in prose",
	"internal/service/access/exhaustive_test.go": "false positive: the checkpoint-package exemption reasons name " +
		"equipment's, insurance's and vitals's own test packages in prose",
	"internal/service/equipment/adapter.go":                "false positive: wiring-error messages and the audit inventory summary name the kind in prose",
	"internal/service/equipment/equipmenttest/fake.go":     "false positive: the fake's own error messages name the kind in prose",
	"internal/service/equipment/service.go":                "false positive: the service's wiring-error messages name the kind in prose",
	"internal/service/equipment/service_test.go":           "false positive, asserting the same messages",
	"internal/service/insurance/adapter.go":                "false positive: wiring-error messages and the audit inventory summary name the kind in prose",
	"internal/service/insurance/insurancetest/fake.go":     "false positive: the fake's own error messages name the kind in prose",
	"internal/service/insurance/service.go":                "false positive: the service's wiring-error messages name the kind in prose",
	"internal/service/insurance/service_test.go":           "false positive, asserting the same messages",
	"internal/store/equipment/mapper.go":                   "false positive: ErrUnexpectedCollection and the mapper's own error messages name the kind in prose",
	"internal/store/equipment/repo_integration_test.go":    "false positive, asserting the same messages",
	"internal/store/insurance/mapper.go":                   "false positive: ErrUnexpectedCollection and the mapper's own error messages (including the coverage/contact JSON errors) name the kind in prose",
	"internal/store/insurance/repo_integration_test.go":    "false positive, asserting the same messages",
	"internal/store/migrations/assertions_test.go":         "false positive: the cascade matrix's own consequence strings name the two kinds in prose",
	"internal/web/api/insurance.go":                        "false positive: a field-error message names insurance's own coverage sub-fields in prose",
	"internal/web/page/equipment.go":                       "false positive: the two page operation ids and the list title name the kind in prose",
	"internal/web/page/insurance.go":                       "false positive: the two page operation ids and the list title name the kind in prose",
	"internal/web/views/records/equipment.go":              "false positive: the view model's field labels name the kind's own fields in prose",
	"internal/web/views/records/equipment_detail_templ.go": "false positive: templ's generated writes of the static markup include the word in aria-label and heading text",
	"internal/web/views/records/equipment_form_templ.go":   "false positive, the same generated-markup shape",
	"internal/web/views/records/equipment_list_templ.go":   "false positive, the same generated-markup shape",
	"internal/web/views/records/equipment_row_templ.go":    "false positive, the same generated-markup shape",
	"internal/web/views/records/insurance.go":              "false positive: the view model's field labels name the kind's own fields in prose",
	"internal/web/views/records/insurance_detail_templ.go": "false positive: templ's generated writes of the static markup include the word in aria-label and heading text",
	"internal/web/views/records/insurance_form_templ.go":   "false positive, the same generated-markup shape",
	"internal/web/views/records/insurance_list_templ.go":   "false positive, the same generated-markup shape",
	"internal/web/views/records/insurance_row_templ.go":    "false positive, the same generated-markup shape",
	"internal/web/api/equipment_contract_test.go":          "false positive: the contract suite's own skip reasons name the kind in prose",
	"internal/web/api/insurance_contract_test.go":          "false positive, the same skip-reason shape",
	"internal/web/api/equipment_http_test.go":              "false positive: an OwnershipCase.Name names the kind in prose",
	"internal/web/api/insurance_http_test.go":              "false positive, the same case-name shape",
	"internal/httproute/routes_test.go": "false positive: the opID and landmark literals for these two kinds' " +
		"pages spell the kind's own name (e.g. \"insuranceListPage\") because the segment equals the kind's " +
		"ordinary English name; every other kind's opID is a distinct plural so this table never needed the exemption before",
	"internal/web/views/records/vitals_list_templ.go": "a false positive: templ embeds its own source " +
		"filename as a literal for its error reporting, and vitals is a mass noun whose singular file " +
		"name (vitals_list.templ) is already its plural collection spelling",
	"internal/web/views/records/vitals_row_templ.go":    "the same, for vitals_row.templ",
	"internal/web/views/records/vitals_detail_templ.go": "the same, for vitals_detail.templ",
	"internal/web/views/records/vitals_form_templ.go":   "the same, for vitals_form.templ",
	"internal/web/api/treatment.go": "FR-028's two multi-relations are named after the collections they point to " +
		"(data-model §4.5); the JSON struct tags carrying those names are literal by construction and cannot call " +
		"Segment() or Collection()",

	// family_member's own "conditions" field (a JSON array of FamilyCondition)
	// spells the unrelated kind.Condition's segment/collection ("conditions")
	// by coincidence of ordinary English, exactly as equipment/insurance
	// collide with their own kind's spelling above — every file below is a
	// field-error message, a JSON key, a struct field or a variable name that
	// says "conditions" because that is what family_member's own sub-field is
	// called, not because it hardcodes a route or a collection.
	"internal/domain/clinical/familycondition.go":            "false positive: FamilyCondition's own field-error messages name family_member's \"conditions\" field in prose",
	"internal/domain/clinical/familymember_test.go":          "false positive, asserting the same field name",
	"internal/service/familymember/adapter.go":               "false positive: the adapter's patch/draft wiring names the \"conditions\" field in prose",
	"internal/store/familymember/mapper.go":                  "false positive: the mapper's own column name and JSON (un)marshal error messages name the \"conditions\" column in prose",
	"internal/store/familymember/repo_integration_test.go":   "false positive, asserting the same column and messages",
	"internal/store/migrations/1756400600_family_members.go": "false positive: the migration names the \"conditions\" JSON column it creates",
	"internal/testsupport/seed/family.go":                    "false positive: the seed's own column constant and JSON encoding name the \"conditions\" column",
	"internal/web/api/familymember.go":                       "false positive: the DTO's own JSON field and field-error messages name \"conditions\" in prose",
	"internal/web/api/familymember_test.go":                  "false positive, asserting the same field name",
	"internal/web/views/records/familymember.go":             "false positive: the view model's field label names family_member's own \"conditions\" field",
	"internal/web/views/records/familymember_templ_test.go":  "false positive, asserting the same field label",
}

func TestNoFileOutsideTheKindTableSpellsAKindSegmentOrCollection(t *testing.T) {
	t.Parallel()

	// One entry per spelling, not per role: segment and collection are the same
	// string today, and a message naming only one of them would send the reader
	// to the wrong accessor.
	spellings := map[string][]string{}
	for _, k := range kind.Kinds() {
		require.NotEmpty(t, k.Segment(), "%s has no segment", k)
		require.NotEmpty(t, k.Collection(), "%s has no collection", k)

		spellings[k.Segment()] = append(spellings[k.Segment()], string(k)+"'s path segment")
		spellings[k.Collection()] = append(spellings[k.Collection()], string(k)+"'s collection")
	}
	require.NotEmpty(t, spellings)

	var (
		offences []string
		scanned  int
	)

	root := repoRoot(t)
	fset := token.NewFileSet()

	walkRepo(t, root, func(rel string, _ fs.DirEntry) {
		if filepath.Ext(rel) != ".go" || rel == kindDeclaration {
			return
		}

		if _, exempt := kindLiteralExempt[rel]; exempt {
			return
		}

		scanned++

		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "parsing %s", rel)

		// Import paths are exempt from the walk below: a package legitimately
		// named after its own kind (internal/store/vitals, say) is imported
		// under that exact path in every file that uses it, and a kind whose
		// segment is a mass noun (vitals, insurance, equipment — singular and
		// plural the same word) makes that path contain the spelling by
		// construction. Nobody hand-writes a route or a collection name as an
		// import path, so this walk has nothing to catch there.
		imports := make(map[*ast.BasicLit]bool, len(file.Imports))
		for _, spec := range file.Imports {
			imports[spec.Path] = true
		}

		ast.Inspect(file, func(node ast.Node) bool {
			// An import path is a Go module path, not a route or a collection
			// name, and no kind whose package is named after its own segment
			// (equipment, insurance: uncountable nouns with no distinct plural)
			// can import itself through Segment() or Collection() — the fix
			// this test otherwise asks for does not exist for this node.
			if _, isImport := node.(*ast.ImportSpec); isImport {
				return false
			}

			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}

			if imports[literal] {
				return true
			}

			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				return true
			}

			// Contained, not equal: `/api/v1/records/medications` is the
			// hardcoded route this exists to stop, and it is never the whole
			// literal.
			for spelling, roles := range spellings {
				if strings.Contains(value, spelling) {
					offences = append(offences, fset.Position(literal.Pos()).String()+
						": the literal spells "+strings.Join(roles, " and ")+
						" — call Segment() or Collection() on the kind (research D-05)")
				}
			}

			return true
		})
	})

	require.Greater(t, scanned, 20, "the walk found almost nothing; it is not looking where it thinks it is")

	sort.Strings(offences)
	assert.Empty(t, offences)
}
