package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// T083. The assertion is on the SQL fragment *and* the bound parameters,
// because either alone would pass a builder that interpolated the value into
// the fragment — which is what PocketBase's own filter DSL does
// (tools/search/filter.go:56-80 substitutes {:param} into the filter text
// before parsing it) and what this package exists to avoid.
func TestTheBuilderProducesTheExpectedSQLAndBoundParameters(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	cases := []struct {
		name       string
		query      Query
		wantWhere  string
		wantParams dbx.Params
		wantOrder  []string
		wantLimit  int
	}{
		{
			name:       "owner scope alone",
			query:      Query{Conditions: []Condition{Equal(medicationFieldOwner, "owner123")}},
			wantWhere:  "([[owner]] = {:mk0})",
			wantParams: dbx.Params{"mk0": "owner123"},
			wantOrder:  []string{"[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name: "narrowed by state, which is FR-022's second half",
			query: Query{Conditions: []Condition{
				Equal(medicationFieldOwner, "owner123"),
				Equal(medicationFieldStatus, string(clinical.TherapyStatusActive)),
			}},
			wantWhere:  "([[owner]] = {:mk0}) AND ([[status]] = {:mk1})",
			wantParams: dbx.Params{"mk0": "owner123", "mk1": "active"},
			wantOrder:  []string{"[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name: "one of several states",
			query: Query{Conditions: []Condition{
				OneOf(medicationFieldStatus, "active", "on_hold"),
			}},
			wantWhere:  "([[status]] IN ({:mk0}, {:mk1}))",
			wantParams: dbx.Params{"mk0": "active", "mk1": "on_hold"},
			wantOrder:  []string{"[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name:       "excluded state",
			query:      Query{Conditions: []Condition{NotEqual(medicationFieldStatus, "cancelled")}},
			wantWhere:  "([[status]] != {:mk0})",
			wantParams: dbx.Params{"mk0": "cancelled"},
			wantOrder:  []string{"[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name:      "a text match against the name, with the wildcards the person typed escaped",
			query:     Query{Conditions: []Condition{Contains(medicationFieldName, "50%_off\\")}},
			wantWhere: `(LOWER([[name]]) LIKE {:mk0} ESCAPE '\')`,
			// The escape character first, or escaping the others would escape
			// the escapes.
			wantParams: dbx.Params{"mk0": `%50\%\_off\\%`},
			wantOrder:  []string{"[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name: "most recently started, which is FR-022's first ordering",
			query: Query{
				Conditions: []Condition{Equal(medicationFieldOwner, "owner123")},
				Sort:       []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}},
				Limit:      10,
			},
			wantWhere:  "([[owner]] = {:mk0})",
			wantParams: dbx.Params{"mk0": "owner123"},
			wantOrder:  []string{"[[started_on]] DESC", "[[id]] DESC"},
			wantLimit:  10,
		},
		{
			// The index is (owner, LOWER(name), id DESC), so the ordering has
			// to be the index's expression or the sort is a filesort.
			name:       "by name, which sorts by the expression the index is built on",
			query:      Query{Sort: []domain.SortKey{{Field: medicationFieldName}}},
			wantWhere:  "",
			wantParams: dbx.Params{},
			wantOrder:  []string{"LOWER([[name]]) ASC", "[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
		{
			name:       "most recently changed",
			query:      Query{Sort: []domain.SortKey{{Field: fieldUpdated, Desc: true}}},
			wantWhere:  "",
			wantParams: dbx.Params{},
			wantOrder:  []string{"[[updated]] DESC", "[[id]] DESC"},
			wantLimit:  DefaultLimit,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := schema.Build(testCase.query)
			require.NoError(t, err)

			params := dbx.Params{}

			var where string
			if built.Where != nil {
				where = built.Where.Build(nil, params)
			}

			assert.Equal(t, testCase.wantWhere, where)
			assert.Equal(t, testCase.wantParams, params)
			assert.Equal(t, testCase.wantOrder, built.OrderBy)
			assert.Equal(t, testCase.wantLimit, built.Limit)
		})
	}
}

// The keyset boundary is a lexicographic row comparison, and it is the whole of
// FR-023: "everything after this row in this ordering" is a predicate a
// concurrent insert cannot change the meaning of, and "skip the first 25 rows"
// is not.
func TestTheKeysetBoundaryIsALexicographicRowComparison(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	cases := []struct {
		name       string
		sort       []domain.SortKey
		after      Cursor
		wantWhere  string
		wantParams dbx.Params
	}{
		{
			name:       "the id alone",
			after:      Cursor{ID: "row000000000001"},
			wantWhere:  "([[id]] < {:mk0})",
			wantParams: dbx.Params{"mk0": "row000000000001"},
		},
		{
			name: "one descending term, with the id as the tiebreaker",
			sort: []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}},
			after: Cursor{
				Sort:   []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}},
				Values: []string{"2026-03-01 00:00:00.000Z"},
				ID:     "row000000000001",
			},
			wantWhere: "([[started_on]] < {:mk0} OR ([[started_on]] = {:mk0} AND [[id]] < {:mk1}))",
			wantParams: dbx.Params{
				"mk0": "2026-03-01 00:00:00.000Z",
				"mk1": "row000000000001",
			},
		},
		{
			name: "one ascending term, where the comparison flips and the tiebreaker does not",
			sort: []domain.SortKey{{Field: medicationFieldName}},
			after: Cursor{
				Sort:   []domain.SortKey{{Field: medicationFieldName}},
				Values: []string{"amoxicillin"},
				ID:     "row000000000001",
			},
			wantWhere: "(LOWER([[name]]) > {:mk0} OR (LOWER([[name]]) = {:mk0} AND [[id]] < {:mk1}))",
			wantParams: dbx.Params{
				"mk0": "amoxicillin",
				"mk1": "row000000000001",
			},
		},
		{
			name: "two terms in opposite directions",
			sort: []domain.SortKey{
				{Field: medicationFieldStatus},
				{Field: fieldUpdated, Desc: true},
			},
			after: Cursor{
				Sort: []domain.SortKey{
					{Field: medicationFieldStatus},
					{Field: fieldUpdated, Desc: true},
				},
				Values: []string{"active", "2026-03-01 09:00:00.000Z"},
				ID:     "row000000000001",
			},
			wantWhere: "([[status]] > {:mk0}" +
				" OR ([[status]] = {:mk0} AND [[updated]] < {:mk1})" +
				" OR ([[status]] = {:mk0} AND [[updated]] = {:mk1} AND [[id]] < {:mk2}))",
			wantParams: dbx.Params{
				"mk0": "active",
				"mk1": "2026-03-01 09:00:00.000Z",
				"mk2": "row000000000001",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := schema.Build(Query{Sort: testCase.sort, After: testCase.after})
			require.NoError(t, err)

			params := dbx.Params{}
			require.NotNil(t, built.Where)

			assert.Equal(t, testCase.wantWhere, built.Where.Build(nil, params))
			assert.Equal(t, testCase.wantParams, params)
		})
	}
}

// D-26: a value outside the published set is refused and never silently
// ignored. A dropped sort term is a different list; a dropped filter term is
// somebody else's rows.
func TestAColumnTheSchemaDoesNotDeclareIsRefused(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	cases := []struct {
		name  string
		query Query
	}{
		{"filtered by an undeclared column", Query{Conditions: []Condition{Equal("secret", "x")}}},
		{"sorted by an undeclared column", Query{Sort: []domain.SortKey{{Field: "secret"}}}},
		{
			name: "paged from a boundary on an undeclared column",
			query: Query{
				Sort:  []domain.SortKey{{Field: "secret"}},
				After: Cursor{Sort: []domain.SortKey{{Field: "secret"}}, Values: []string{"x"}, ID: "row000000000001"},
			},
		},
		{
			// Free text a person wrote about their own health. Nothing in this
			// phase filters or sorts by it, and an allowlist that lists
			// everything is not one.
			name:  "filtered by a free-text clinical column",
			query: Query{Conditions: []Condition{Contains(medicationFieldNotes, "x")}},
		},
		{"sorted by a free-text clinical column", Query{Sort: []domain.SortKey{{Field: medicationFieldSideEffects}}}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := schema.Build(testCase.query)
			assert.ErrorIs(t, err, ErrUnknownColumn)
		})
	}
}

func TestAQueryTheBuilderCannotHonourIsRefused(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	cases := []struct {
		name  string
		query Query
	}{
		{"a limit above the published maximum", Query{Limit: MaxLimit + 1}},
		{"a negative limit", Query{Limit: -1}},
		{"one of nothing", Query{Conditions: []Condition{OneOf(medicationFieldStatus)}}},
		{"an operator that is not one", Query{Conditions: []Condition{{Column: medicationFieldStatus, Op: "regex", Values: []string{"x"}}}}},
		{
			name: "a boundary whose ordering is not the one being paged",
			query: Query{
				Sort:  []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}},
				After: Cursor{Sort: []domain.SortKey{{Field: medicationFieldName}}, Values: []string{"x"}, ID: "row000000000001"},
			},
		},
		{
			// Not the zero cursor, which is the first page: a boundary that
			// names an ordering and a value but no row.
			name: "a boundary with no id, which is a boundary with no tiebreaker",
			query: Query{
				Sort:  []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}},
				After: Cursor{Sort: []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}}, Values: []string{"x"}},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := schema.Build(testCase.query)
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrUnknownColumn, "the column was fine; something else was wrong")
		})
	}
}

// The one that would otherwise rot silently. A column declares two things — the
// SQL it is ordered and compared by, and how to read the same value out of a
// record in Go — and the keyset only works while they agree. LOWER() in SQLite
// folds ASCII and nothing else; strings.ToLower folds the whole of Unicode. A
// boundary computed with the wrong one lands in the wrong place in the sequence
// and the page silently skips rows.
func TestEveryDeclaredColumnsGoValueIsWhatSQLiteComputesForIt(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "columns@example.test")

	schema := MedicationSchema()

	// Names picked so a case-folding disagreement shows up: a Turkish dotted
	// capital I folds to two code points in Go and not at all in SQLite.
	for _, name := range []string{"Amoxicillin", "IBUPROFEN", "İnsulin", "リン酸コデイン", "50% off"} {
		medication := sampleMedication(t, owner.Id)
		medication.Name = name
		seedMedication(t, app, medication)
	}

	records, err := app.FindAllRecords(kind.Medication.Collection())
	require.NoError(t, err)
	require.Len(t, records, 5)

	for _, column := range schema.Columns() {
		declared, ok := schema.Column(column)
		require.True(t, ok)

		t.Run(column, func(t *testing.T) {
			t.Parallel()

			for _, record := range records {
				var fromSQLite string

				require.NoError(t, app.DB().
					Select(declared.Expr).
					From(kind.Medication.Collection()).
					Where(dbxID(record.Id)).
					Row(&fromSQLite))

				assert.Equalf(t, fromSQLite, declared.Value(record),
					"%s: the Go value and the SQL expression disagree, so a keyset boundary on this column lands in the wrong place",
					column)
			}
		})
	}
}

// FR-023, end to end and against a real database. This is the requirement in
// one test: page through a list, have somebody insert a row into the part of it
// that has already gone past, and see neither a repeat nor a gap.
//
// An OFFSET cursor fails this. It is defined against a result set that is
// changing underneath it, so the row inserted above the boundary pushes every
// later page along by one and the reader silently never sees one entry.
func TestPagingFromAKeysetBoundaryNeitherRepeatsNorSkipsWhenARowIsInserted(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "paging@example.test")

	schema := MedicationSchema()
	ordering := []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}}

	// Six rows over five days, two of them sharing a date so the id tiebreaker
	// is actually exercised rather than merely present.
	for _, seed := range []struct{ name, started string }{
		{"Amoxicillin", "2026-03-06"},
		{"Bisoprolol", "2026-03-05"},
		{"Ciclosporin", "2026-03-04"},
		{"Dapagliflozin", "2026-03-04"},
		{"Enalapril", "2026-03-03"},
		{"Furosemide", "2026-03-02"},
	} {
		medication := sampleMedication(t, owner.Id)
		medication.Name = seed.name
		medication.StartedOn = mustDate(t, seed.started)
		medication.EndedOn = domain.Date{}
		seedMedication(t, app, medication)
	}

	page := func(after Cursor) ([]clinical.Medication, Cursor) {
		t.Helper()

		built, err := schema.Build(Query{
			Conditions: []Condition{Equal(medicationFieldOwner, owner.Id)},
			Sort:       ordering,
			After:      after,
			Limit:      2,
		})
		require.NoError(t, err)

		var records []*core.Record
		require.NoError(t, built.Apply(app.RecordQuery(kind.Medication.Collection())).All(&records))

		items := make([]clinical.Medication, 0, len(records))
		for _, record := range records {
			medication, mapErr := MedicationFromRecord(record)
			require.NoError(t, mapErr)
			items = append(items, medication)
		}

		if len(records) == 0 {
			return items, Cursor{}
		}

		next, err := schema.Boundary(records[len(records)-1], ordering)
		require.NoError(t, err)

		return items, next
	}

	var seen []string

	first, cursor := page(Cursor{})
	require.Len(t, first, 2)
	for _, item := range first {
		seen = append(seen, item.Name)
	}
	require.Equal(t, []string{"Amoxicillin", "Bisoprolol"}, seen)

	// The concurrent insert, deliberately above the boundary: a newer start
	// date than anything already read, which is where an offset loses a row.
	inserted := sampleMedication(t, owner.Id)
	inserted.Name = "Zopiclone"
	inserted.StartedOn = mustDate(t, "2026-03-09")
	inserted.EndedOn = domain.Date{}
	seedMedication(t, app, inserted)

	for range 5 {
		items, next := page(cursor)
		if len(items) == 0 {
			break
		}

		for _, item := range items {
			seen = append(seen, item.Name)
		}

		cursor = next
	}

	// Set equality *and* length: together they say no row was repeated and none
	// was skipped, which is FR-023 exactly. The two rows sharing 2026-03-04 are
	// ordered between themselves by the id tiebreaker, and an id is fifteen
	// random characters — so their relative order is not a thing to assert.
	assert.ElementsMatch(t,
		[]string{"Amoxicillin", "Bisoprolol", "Ciclosporin", "Dapagliflozin", "Enalapril", "Furosemide"},
		seen,
		"the reader either saw a row twice or never saw one at all")
	assert.Len(t, seen, 6)

	// The ordering still holds either side of the tie.
	require.Len(t, seen, 6)
	assert.Equal(t, []string{"Amoxicillin", "Bisoprolol"}, seen[:2])
	assert.ElementsMatch(t, []string{"Ciclosporin", "Dapagliflozin"}, seen[2:4])
	assert.Equal(t, []string{"Enalapril", "Furosemide"}, seen[4:])

	// And the row inserted above the boundary is genuinely there — so the
	// assertion above is "it was not shown to a reader who had already gone
	// past its position", not "it was never written".
	total, err := app.CountRecords(kind.Medication.Collection())
	require.NoError(t, err)
	assert.EqualValues(t, 7, total)
}

// The same paging, with two rows sharing a sort value, which is the case the id
// tiebreaker exists for. Without it a page boundary in the middle of a tie
// either repeats the tied rows or drops them.
func TestPagingIsStableAcrossATieInTheSortColumn(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "ties@example.test")

	schema := MedicationSchema()
	ordering := []domain.SortKey{{Field: medicationFieldStartedOn, Desc: true}}

	for _, name := range []string{"Amoxicillin", "Bisoprolol", "Ciclosporin", "Dapagliflozin"} {
		medication := sampleMedication(t, owner.Id)
		medication.Name = name
		medication.StartedOn = mustDate(t, "2026-03-04")
		medication.EndedOn = domain.Date{}
		seedMedication(t, app, medication)
	}

	seen := map[string]int{}
	cursor := Cursor{}

	for range 6 {
		built, err := schema.Build(Query{
			Conditions: []Condition{Equal(medicationFieldOwner, owner.Id)},
			Sort:       ordering,
			After:      cursor,
			Limit:      1,
		})
		require.NoError(t, err)

		var records []*core.Record
		require.NoError(t, built.Apply(app.RecordQuery(kind.Medication.Collection())).All(&records))

		if len(records) == 0 {
			break
		}

		for _, record := range records {
			seen[record.Id]++
		}

		cursor, err = schema.Boundary(records[len(records)-1], ordering)
		require.NoError(t, err)
	}

	require.Len(t, seen, 4, "a tied row was skipped")
	for id, count := range seen {
		assert.Equalf(t, 1, count, "%s was returned on more than one page", id)
	}
}

func TestTheLimitIsDefaultedAndBounded(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	assert.Equal(t, 25, DefaultLimit, "research D-25 publishes this number")
	assert.Equal(t, 100, MaxLimit)

	defaulted, err := schema.Build(Query{})
	require.NoError(t, err)
	assert.Equal(t, DefaultLimit, defaulted.Limit)

	atMax, err := schema.Build(Query{Limit: MaxLimit})
	require.NoError(t, err)
	assert.Equal(t, MaxLimit, atMax.Limit)
}

// The allowlist is a decision, so it is written down where a change to it shows
// up as a change to this line rather than as a wider query surface nobody
// noticed. Every column here is one FR-022 names or one the cursor needs;
// everything absent is free text a person wrote about their own health.
func TestTheQueryableColumnsAreExactlyTheOnesTheRequirementsName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		fieldID,
		medicationFieldOwner,
		medicationFieldName,
		medicationFieldType,
		medicationFieldRoute,
		medicationFieldStatus,
		medicationFieldStartedOn,
		medicationFieldEndedOn,
		fieldCreated,
		fieldUpdated,
	}, MedicationSchema().Columns())

	assert.Equal(t, kind.Medication.Collection(), MedicationSchema().Collection())
}

// ---------------------------------------------------------------------------
// The source walk
// ---------------------------------------------------------------------------

// Files outside internal/store that may hold something this walk would
// otherwise flag, each because the string is the point of the file.
var filterDSLExempt = map[string]string{
	"internal/platform/pb/assert_test.go":    "the negative fixture: it sets an API rule precisely to prove the boot assertion refuses one",
	"internal/platform/pb/adminwarn_test.go": "the negative fixture: it sets a partial MFA rule precisely to prove the boot warning fires on one",
}

// PocketBase's own entry points into its filter DSL. A call to one of these is
// a filter string by definition, wherever the string itself was written.
//
// The literal detector below cannot see a filter that was assembled rather than
// written, so this list is the whole backstop for one.
var filterDSLCalls = map[string]string{
	"FindRecordsByFilter":     "takes a filter DSL string and substitutes its {:params} into the text before parsing (tools/search/filter.go:56-80)",
	"FindFirstRecordByFilter": "the same, for one record",
	"CountRecordsByFilter":    "the same, for a count",
	"FilterData":              "the DSL parser itself",
	"ParseSortFromString":     "the sort half of the same DSL",
	"NewRecordFieldResolver":  "resolves DSL identifiers against a collection, which only a DSL string needs",
	"CanAccessRecord":         "runs the rule it is handed through the DSL — search.FilterData(*accessRule).BuildExpr(resolver) at core/record_query.go:620-621 — so an access rule assembled at the call site is a filter (research D-26)",
	"ParseAndExec":            "search.Provider's one-shot: it reads the raw `filter=` URL query parameter straight into the DSL (tools/search/provider.go:224-226)",
}

// Names PocketBase's search provider shares with the standard library. Banned
// on the same terms as the list above, but not when the receiver is a
// standard-library package this file imports: `Parse` is search.Provider's
// front door onto the raw `filter=` query parameter and it is also url.Parse
// and time.Parse, and a gate that flagged those would be turned off within a
// week.
var filterDSLAmbiguousCalls = map[string]string{
	"Parse": "search.Provider.Parse reads the raw `filter=` URL query parameter into the DSL (tools/search/provider.go:224-226) — request input straight into a query language",
}

var (
	// {:param} — a PocketBase filter placeholder, which is not a SQL bind
	// parameter: the value is substituted into the filter *text*.
	filterPlaceholder = regexp.MustCompile(`\{:\w+\}`)

	// `field OP 'value'` — a comparison against a quoted operand, which is the
	// shape a written-out filter takes and is not a shape Go prose takes.
	filterComparison = regexp.MustCompile(`^\s*[A-Za-z_][A-Za-z0-9_.]*\s*(!=|>=|<=|!~|=|~|>|<)\s*['"]`)
)

// fexpr's any/multi-match operators. They exist in no other language MediKube
// writes, so one of them in a string literal is a filter and nothing else.
var filterMultiMatchOperators = []string{"?=", "?!=", "?~", "?!~", "?<=", "?>=", "?<", "?>"}

// T083's second half. `internal/store` is where PocketBase's filter DSL is
// written and it is where it stays: everywhere else builds a typed Query and
// lets this package turn it into bound parameters.
//
// The reason is not tidiness. A DSL string is a second query language with its
// own parser, its own operator set and its own substitution rules, and the one
// place a value gets interpolated into a query text rather than bound to it. A
// filter assembled from a request in a handler is that interpolation happening
// somewhere nobody is looking.
//
// Nothing else enforces this. depguard bans packages, not strings; forbidigo's
// patterns are function names. This walk is the whole gate, which is why it
// checks the call sites as well as the literals.
func TestNoFilterDSLStringAppearsOutsideThisPackage(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fileSet := token.NewFileSet()

	var (
		offences []string
		scanned  int
	)

	walkGoFiles(t, root, func(rel string) {
		if filterDSLWalkSkips(rel) {
			return
		}

		if _, exempt := filterDSLExempt[rel]; exempt {
			return
		}

		scanned++

		offences = append(offences, filterDSLOffences(t, fileSet, filepath.Join(root, rel))...)
	})

	require.Greater(t, scanned, 20, "the walk found almost nothing; it is not looking where it thinks it is")

	sort.Strings(offences)
	assert.Empty(t, offences)
}

// The package that writes the DSL, and the one tree under it that legitimately
// restores PocketBase's own stock API rules.
const (
	filterDSLPackage    = "internal/store"
	filterDSLMigrations = "internal/store/migrations"
)

// filterDSLWalkSkips is the exemption, and it is a package and a tree rather
// than a prefix. `internal/store/` as a prefix also exempted
// internal/store/medication, internal/store/identity and internal/store/audit —
// the three repository packages phases 002-006 fill, doc.go only today — which
// is precisely the population this gate exists to police. The comment justified
// migrations and the code exempted four packages more.
func filterDSLWalkSkips(rel string) bool {
	dir := path.Dir(rel)

	return dir == filterDSLPackage ||
		dir == filterDSLMigrations ||
		strings.HasPrefix(dir, filterDSLMigrations+"/")
}

// filterDSLOffences is one file's findings, factored out of the walk so the
// detector can be run over a source that does not exist in the repository —
// which is the only way to prove a repository package that has not been written
// yet would be caught.
func filterDSLOffences(t *testing.T, fileSet *token.FileSet, absolute string) []string {
	t.Helper()

	file, err := parser.ParseFile(fileSet, absolute, nil, parser.SkipObjectResolution)
	require.NoErrorf(t, err, "parsing %s", absolute)

	stdlib := standardLibraryImports(file)

	var offences []string

	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			reason, banned := filterDSLCalls[typed.Sel.Name]
			if !banned {
				if ambiguous, shared := filterDSLAmbiguousCalls[typed.Sel.Name]; shared && !stdlibReceiver(typed.X, stdlib) {
					reason, banned = ambiguous, true
				}
			}

			if banned {
				offences = append(offences, fileSet.Position(typed.Pos()).String()+
					": names "+typed.Sel.Name+", which "+reason+
					" — build a store.Query instead (research D-26, plan.md internal/store)")
			}
		case *ast.BasicLit:
			if typed.Kind != token.STRING {
				return true
			}

			value, unquoteErr := strconv.Unquote(typed.Value)
			if unquoteErr != nil {
				return true
			}

			if reason := looksLikeFilterDSL(value); reason != "" {
				offences = append(offences, fileSet.Position(typed.Pos()).String()+
					": the literal is a PocketBase filter expression ("+reason+
					") — build a store.Query instead, or add the file to filterDSLExempt with a reason")
			}
		}

		return true
	})

	return offences
}

// standardLibraryImports is the local names this file binds to standard-library
// packages. A module path always carries a dot in its first element and a
// standard-library path never does, which is the same test the architecture
// suite uses and costs no dependency.
func standardLibraryImports(file *ast.File) map[string]bool {
	names := map[string]bool{}

	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		first, rest, _ := strings.Cut(importPath, "/")
		if strings.Contains(first, ".") {
			continue
		}

		local := path.Base(first + "/" + rest)
		if spec.Name != nil {
			local = spec.Name.Name
		}

		names[local] = true
	}

	return names
}

// stdlibReceiver is true for `url.Parse` and false for `provider.Parse`. A
// local variable shadowing an imported package name would read as the package;
// that is a narrower hole than banning the name outright would open.
func stdlibReceiver(receiver ast.Expr, stdlib map[string]bool) bool {
	ident, isIdent := receiver.(*ast.Ident)

	return isIdent && stdlib[ident.Name]
}

func looksLikeFilterDSL(value string) string {
	switch {
	case strings.Contains(value, "@request."):
		return "it names @request, which only PocketBase's rule and filter grammar has"
	case strings.Contains(value, "@collection."):
		return "it names @collection, which only PocketBase's rule and filter grammar has"
	case filterPlaceholder.MatchString(value):
		return "it carries a {:param} placeholder, which is substituted into the filter text rather than bound"
	}

	for _, operator := range filterMultiMatchOperators {
		if strings.Contains(value, operator) {
			return "it uses fexpr's " + operator + " any-match operator"
		}
	}

	if filterComparison.MatchString(value) {
		return "it compares a bare identifier against a quoted operand"
	}

	return ""
}

// The exemption is the other half of the gate, and it is the half that fails
// silently: a file the walk never opens is a file with no findings.
func TestTheSourceWalkExemptsThisPackageAndMigrationsAndNothingElse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rel     string
		skipped bool
		why     string
	}{
		{rel: "internal/store/filter.go", skipped: true, why: "this package is where the DSL is written"},
		{rel: "internal/store/cursor.go", skipped: true, why: "the same package"},
		{rel: "internal/store/filter_test.go", skipped: true, why: "this file names every banned call by hand"},
		{rel: "internal/store/migrations/1756100200_" + kind.Medication.Collection() + ".go", skipped: true, why: "migrations restore PocketBase's own stock API rules"},
		{rel: "internal/store/medication/repo.go", skipped: false, why: "a repository package: exactly where an assembled filter would be written"},
		{rel: "internal/store/identity/repo.go", skipped: false, why: "the same"},
		{rel: "internal/store/audit/repo.go", skipped: false, why: "the same"},
		{rel: "internal/web/api/" + kind.Medication.Collection() + ".go", skipped: false, why: "a handler, which is where request input meets a query"},
		{rel: "internal/storefront/query.go", skipped: false, why: "a prefix match on internal/store would take this too"},
	}

	for _, testCase := range cases {
		t.Run(testCase.rel, func(t *testing.T) {
			t.Parallel()

			assert.Equalf(t, testCase.skipped, filterDSLWalkSkips(testCase.rel), "%s — %s", testCase.rel, testCase.why)
		})
	}
}

// And the file the exemption above no longer covers has to actually produce a
// finding, which nothing proves while internal/store/medication holds a doc.go
// and nothing else. So the detector is run over a repository package written
// for the purpose.
func TestARepositoryPackageThatReachesForTheDSLIsAFinding(t *testing.T) {
	t.Parallel()

	// The collection is spelled by the kind table and not by hand, which is the
	// rule internal/architecture's kind-literal walk enforces on every file.
	repo := `package medication

import (
	"net/url"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)

func List(app core.App, owner string, provider *search.Provider, raw string) error {
	if _, err := url.Parse(raw); err != nil {
		return err
	}

	if err := provider.Parse(raw); err != nil {
		return err
	}

	if _, err := provider.ParseAndExec(raw, nil); err != nil {
		return err
	}

	if ok, err := app.CanAccessRecord(nil, nil, nil); err != nil || !ok {
		return err
	}

	_, err := app.FindRecordsByFilter("` + kind.Medication.Collection() + `", "owner = {:owner}", "-created", 25, 0)

	return err
}
`

	absolute := filepath.Join(t.TempDir(), "repo.go")
	require.NoError(t, os.WriteFile(absolute, []byte(repo), 0o600))

	require.False(t, filterDSLWalkSkips("internal/store/medication/repo.go"),
		"the walk has to open the file before the detector can say anything about it")

	offences := filterDSLOffences(t, token.NewFileSet(), absolute)

	var reported []string

	for _, offence := range offences {
		if named := filterDSLReportedCall.FindStringSubmatch(offence); named != nil {
			reported = append(reported, named[1])
		}
	}

	// Exactly these, and `Parse` exactly once: url.Parse is in the same file,
	// and a gate that flagged it would be paid for in exemptions until somebody
	// deleted it.
	assert.ElementsMatch(t,
		[]string{"FindRecordsByFilter", "CanAccessRecord", "ParseAndExec", "Parse"},
		reported)

	assert.Contains(t, strings.Join(offences, "\n"), "{:param} placeholder",
		"the written-out filter in the same file is still a finding of its own")
}

// The call named by an offence, so a test can assert the exact set rather than
// that each name appears somewhere in a megabyte of message.
var filterDSLReportedCall = regexp.MustCompile(`: names (\w+), which `)

// The gate has to be able to fail, so this is the same detector run against
// strings that are filters. If a change to looksLikeFilterDSL stops it seeing
// these, the walk above would go quiet rather than go red.
func TestTheSourceWalkRecognisesAFilterWhenItSeesOne(t *testing.T) {
	t.Parallel()

	filters := []string{
		"id = @request.auth.id",
		"owner = {:owner}",
		"name ~ {:query}",
		"status ?= 'active'",
		"tags ?~ 'x'",
		`name = 'Amoxicillin'`,
		`owner.email = "a@b.test"`,
		"started_on >= '2026-03-01'",
	}

	for _, filter := range filters {
		assert.NotEmptyf(t, looksLikeFilterDSL(filter), "%q was not recognised as a filter", filter)
	}

	// And it has to stay quiet on the things MediKube's own source is full of,
	// or the exemption list becomes the real gate.
	notFilters := []string{
		"",
		kind.Medication.Collection(),
		"the record was updated",
		"sql IS NOT NULL AND name NOT LIKE 'sqlite_autoindex_%'",
		"GET /api/collections/{collection}/records",
		"application/json",
		"want != got",
		"https://example.test/path?a=b",
		"2026-03-01 00:00:00.000Z",
		"%!s(MISSING)",
		"a == b",
	}

	for _, plain := range notFilters {
		assert.Emptyf(t, looksLikeFilterDSL(plain), "%q was mistaken for a filter", plain)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "walked to the filesystem root without finding go.mod")
		dir = parent
	}
}

func walkGoFiles(t *testing.T, root string, visit func(rel string)) {
	t.Helper()

	skip := map[string]bool{"node_modules": true, "pb_data": true}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		if entry.IsDir() {
			if path != root && (skip[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return fs.SkipDir
			}

			return nil
		}

		if filepath.Ext(rel) == ".go" {
			visit(filepath.ToSlash(rel))
		}

		return nil
	})
	require.NoError(t, err)
}

// The SQL the builder writes has to be SQL SQLite accepts, and no unit
// assertion on a fragment can say that: the [[column]] quoting, the {:param}
// binding and the ESCAPE clause are all resolved by dbx and the driver long
// after Build has returned.
func TestTheBuiltQueryRunsAndNarrowsTheWayItSaysItDoes(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "narrow@example.test")
	stranger := seedUser(t, app, "stranger@example.test")

	for _, seed := range []struct {
		ownerID string
		name    string
		status  clinical.TherapyStatus
	}{
		{owner.Id, "Amoxicillin", clinical.TherapyStatusActive},
		{owner.Id, "AMOXICILLIN clavulanate", clinical.TherapyStatusActive},
		{owner.Id, "Bisoprolol", clinical.TherapyStatusStopped},
		{owner.Id, "50% dextrose", clinical.TherapyStatusActive},
		{owner.Id, "dextrose 5", clinical.TherapyStatusActive},
		{stranger.Id, "Amoxicillin", clinical.TherapyStatusActive},
	} {
		medication := sampleMedication(t, seed.ownerID)
		medication.Name = seed.name
		medication.Status = seed.status
		seedMedication(t, app, medication)
	}

	schema := MedicationSchema()

	run := func(t *testing.T, query Query) []string {
		t.Helper()

		built, err := schema.Build(query)
		require.NoError(t, err)

		var records []*core.Record
		require.NoError(t, built.Apply(app.RecordQuery(kind.Medication.Collection())).All(&records))

		names := make([]string, 0, len(records))
		for _, record := range records {
			names = append(names, record.GetString(medicationFieldName))
		}

		return names
	}

	cases := []struct {
		name  string
		query Query
		want  []string
	}{
		{
			name: "the owner scope, which is the only thing between two people's records",
			query: Query{
				Conditions: []Condition{Equal(medicationFieldOwner, owner.Id)},
				Sort:       []domain.SortKey{{Field: medicationFieldName}},
			},
			want: []string{"50% dextrose", "Amoxicillin", "AMOXICILLIN clavulanate", "Bisoprolol", "dextrose 5"},
		},
		{
			name: "narrowed by state",
			query: Query{
				Conditions: []Condition{
					Equal(medicationFieldOwner, owner.Id),
					Equal(medicationFieldStatus, string(clinical.TherapyStatusStopped)),
				},
			},
			want: []string{"Bisoprolol"},
		},
		{
			name: "one of several states",
			query: Query{
				Conditions: []Condition{
					Equal(medicationFieldOwner, owner.Id),
					OneOf(medicationFieldStatus, string(clinical.TherapyStatusStopped), string(clinical.TherapyStatusCompleted)),
				},
			},
			want: []string{"Bisoprolol"},
		},
		{
			name: "a text match, which folds ASCII case because SQLite's LIKE does",
			query: Query{
				Conditions: []Condition{
					Equal(medicationFieldOwner, owner.Id),
					Contains(medicationFieldName, "amox"),
				},
				Sort: []domain.SortKey{{Field: medicationFieldName}},
			},
			want: []string{"Amoxicillin", "AMOXICILLIN clavulanate"},
		},
		{
			// The wildcard the person typed is a character they are searching
			// for, not a request for everything.
			name: "a text match containing a per-cent sign",
			query: Query{
				Conditions: []Condition{
					Equal(medicationFieldOwner, owner.Id),
					Contains(medicationFieldName, "50%"),
				},
			},
			want: []string{"50% dextrose"},
		},
		{
			name: "a text match containing an underscore, the other wildcard",
			query: Query{
				Conditions: []Condition{
					Equal(medicationFieldOwner, owner.Id),
					Contains(medicationFieldName, "e_5"),
				},
			},
			want: []string{},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, run(t, testCase.query))
		})
	}
}
