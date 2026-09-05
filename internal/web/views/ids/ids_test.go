package ids_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/ids"
)

// A DOM identifier that reaches a Datastar selector as "#"+id. Anything outside
// this shape either needs CSS escaping — which web.WithSelectorID does not do
// and cannot, because it is handed a bare id — or, unescaped in an attribute,
// closes the attribute and opens an element.
var validID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// The record id shapes that must not be able to reach the DOM intact. The first
// is what a PocketBase id actually looks like; the rest are what an id is if
// something upstream of here ever stops checking.
var recordIDs = []struct {
	name string
	id   string
}{
	{"a PocketBase id", "mkmedamara00003"},
	{"a space", "two words"},
	{"a quote and an attribute", `x" onload="alert(1)`},
	{"a selector metacharacter", "a.b#c"},
	{"a closing tag", "</td><script>"},
	{"a newline", "a\nb"},
	{"a non-ASCII rune", "مسكن"},
}

// Every id-producing function in the package, so a new one added without a
// shape is an offence here rather than a selector nobody tried.
func kindIDs(k kind.Kind) map[string]string {
	return map[string]string{
		"RecordList":    ids.RecordList(k),
		"RecordRows":    ids.RecordRows(k),
		"RecordEmpty":   ids.RecordEmpty(k),
		"RecordPager":   ids.RecordPager(k),
		"RecordRow":     ids.RecordRow(k, "mkmedamara00003"),
		"RecordDetail":  ids.RecordDetail(k, "mkmedamara00003"),
		"RecordForm":    ids.RecordForm(k, "mkmedamara00003"),
		"RecordCreate":  ids.RecordForm(k, ""),
		"RecordConfirm": ids.RecordConfirm(k, "mkmedamara00003"),
		"RecordBasis":   ids.RecordBasis(k, "mkmedamara00003"),
	}
}

func TestEveryIDIsAUsableSelector(t *testing.T) {
	t.Parallel()

	for _, k := range kind.Kinds() {
		for name, id := range kindIDs(k) {
			t.Run(string(k)+" "+name, func(t *testing.T) {
				t.Parallel()

				assert.Regexp(t, validID, id, "%s is patched by #%s and a selector cannot hold that", name, id)
			})
		}
	}

	for name, id := range map[string]string{
		"Main":          ids.Main,
		"ErrorBanner":   ids.ErrorBanner,
		"Toast":         ids.Toast,
		"Field":         ids.Field("medication-form", "alternative_name"),
		"FieldError":    ids.FieldError("medication-form", "alternative_name"),
		"Criteria":      ids.Criteria("search"),
		"CriteriaChip0": ids.CriteriaChip("search", 0),
		"CriteriaChip1": ids.CriteriaChip("search", 1),
	} {
		t.Run("shell "+name, func(t *testing.T) {
			t.Parallel()

			assert.Regexp(t, validID, id)
		})
	}
}

// The whole reason the package exists: the templ component writes the id and
// the stream writes the selector, and both call this.
func TestTheSameArgumentsAlwaysProduceTheSameID(t *testing.T) {
	t.Parallel()

	for _, k := range kind.Kinds() {
		for _, record := range recordIDs {
			t.Run(string(k)+" "+record.name, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, ids.RecordRow(k, record.id), ids.RecordRow(k, record.id))
			})
		}
	}
}

func TestARecordIDThatIsNotAnIdentifierCannotReachTheDOM(t *testing.T) {
	t.Parallel()

	for _, record := range recordIDs {
		t.Run(record.name, func(t *testing.T) {
			t.Parallel()

			for name, id := range map[string]string{
				"RecordRow":     ids.RecordRow(kind.Medication, record.id),
				"RecordDetail":  ids.RecordDetail(kind.Medication, record.id),
				"RecordForm":    ids.RecordForm(kind.Medication, record.id),
				"RecordConfirm": ids.RecordConfirm(kind.Medication, record.id),
				"RecordBasis":   ids.RecordBasis(kind.Medication, record.id),
			} {
				assert.Regexp(t, validID, id, "%s let a hostile id through", name)
			}
		})
	}
}

// Two records, two rows. A row id that collapsed onto another record's would
// make the stream patch the wrong medication into view.
func TestDifferentRecordsAndDifferentKindsGetDifferentIDs(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}

	for _, k := range kind.Kinds() {
		for name, id := range kindIDs(k) {
			where := string(k) + "." + name
			previous, clash := seen[id]
			require.Falsef(t, clash, "%s and %s are both %q", previous, where, id)
			seen[id] = where
		}
	}

	assert.NotEqual(t,
		ids.RecordRow(kind.Medication, "mkmedamara00003"),
		ids.RecordRow(kind.Medication, "mkmedamara00004"))
}

// research D-05: the spelling is declared once. An id built from a literal
// would be the fourth spelling of a kind, and the one nothing checks.
func TestTheIDIsBuiltFromTheKindTable(t *testing.T) {
	t.Parallel()

	for _, k := range kind.Kinds() {
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()

			for name, id := range kindIDs(k) {
				assert.Truef(t, strings.HasPrefix(id, k.Enum()),
					"%s is %q and does not start with the kind's own spelling", name, id)
			}
		})
	}
}

// A kind the table does not declare has no spelling to build on. The id still
// has to be a selector, because the alternative is an unpatchable element and a
// console warning rather than a failure.
func TestAnUndeclaredKindStillProducesASelector(t *testing.T) {
	t.Parallel()

	for _, k := range []kind.Kind{"", " ", "not_declared"} {
		t.Run("kind "+string(k), func(t *testing.T) {
			t.Parallel()

			for name, id := range kindIDs(k) {
				assert.Regexpf(t, validID, id, "%s", name)
			}
		})
	}
}

func TestFieldAndItsErrorAreLinkableAndDistinct(t *testing.T) {
	t.Parallel()

	fields := []string{"name", "alternative_name", "started_on", "side_effects"}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			control := ids.Field("medication-form", field)
			message := ids.FieldError("medication-form", field)

			assert.Regexp(t, validID, control)
			assert.Regexp(t, validID, message)
			assert.NotEqual(t, control, message,
				"aria-describedby must point at the message and not at the control itself")
		})
	}

	assert.NotEqual(t, ids.Field("medication-form", "name"), ids.Field("medication-create", "name"),
		"two forms on one page would otherwise share a control id")
}

func TestCriteriaChipsAreDistinctPerIndexAndPerScope(t *testing.T) {
	t.Parallel()

	assert.NotEqual(t, ids.CriteriaChip("search", 0), ids.CriteriaChip("search", 1))
	assert.NotEqual(t, ids.CriteriaChip("search", 0), ids.CriteriaChip("timeline", 0))
	assert.Equal(t, ids.CriteriaChip("search", 0), ids.CriteriaChip("search", 0))
}
