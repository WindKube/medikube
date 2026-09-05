package store

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// research D-25 publishes both numbers. They live here as well as at the edge
// because the edge validates a request and this is the last line: a limit that
// got past the edge is a bug, not a large page.
const (
	DefaultLimit = 25
	MaxLimit     = 100
)

var (
	// ErrUnknownColumn is a column the schema does not declare. It is an error
	// rather than a term that gets dropped: a dropped sort term is a different
	// list and a dropped filter term is somebody else's rows (research D-26).
	ErrUnknownColumn = errors.New("store: the query names a column this resource does not publish")

	// ErrInvalidQuery is a query that is well-typed and still not answerable.
	ErrInvalidQuery = errors.New("store: the query cannot be built as asked")
)

// Operator is the comparison a condition makes. The set is closed, which is
// half the point of the package: PocketBase's own filter grammar has eighteen
// operators including four kinds of any-match, and every one of them is a way
// to ask a question this resource did not offer.
type Operator string

const (
	OpEqual    Operator = "equal"
	OpNotEqual Operator = "not_equal"
	OpOneOf    Operator = "one_of"
	OpContains Operator = "contains"
	// OpGTE and OpLTE are the two halves of a date range (`?from=`/`?to=`,
	// research D-05): each is one term, so a range is two Conditions ANDed by
	// Build, never a single term neither half of this package's callers can
	// spell as a filter string.
	OpGTE Operator = "gte"
	OpLTE Operator = "lte"
	// OpAnyOf and OpAllOf are `?tags=&match=any|all`: membership in a
	// MaxSelect:0 relation column, which PocketBase stores as a JSON array of
	// ids in one text column. Every id is bound as its own parameter and
	// compared with LIKE against the escaped JSON fragment — never IN, which
	// tests scalar equality and would never match a multi-valued column.
	OpAnyOf Operator = "any_of"
	OpAllOf Operator = "all_of"
)

// Condition is one narrowing term. Build them with the constructors below
// rather than by hand — the shape of Values depends on the operator, and the
// constructors are what make that unrepresentable at the call site.
type Condition struct {
	// Columns is the column the term compares, or — for a search that spans
	// more than one — the columns it holds against ANY of. Several columns are
	// a disjunction inside the term and still a conjunction with every other
	// term, which is the only shape FR-022's `?q=` needs: one value, two
	// columns, and nothing that lets a caller nest anything.
	Columns []string
	Op      Operator
	Values  []string
}

func Equal(column, value string) Condition {
	return Condition{Columns: []string{column}, Op: OpEqual, Values: []string{value}}
}

func NotEqual(column, value string) Condition {
	return Condition{Columns: []string{column}, Op: OpNotEqual, Values: []string{value}}
}

func OneOf(column string, values ...string) Condition {
	return Condition{Columns: []string{column}, Op: OpOneOf, Values: values}
}

// Contains is FR-022's "text match against the name".
//
// SQLite's LIKE folds ASCII case and nothing else, so this matches
// case-insensitively for Latin text and exactly for everything else. That is a
// property of the database, not a decision made here, and it is why the name
// column sorts by LOWER() but matches by LIKE.
func Contains(column, value string) Condition {
	return Condition{Columns: []string{column}, Op: OpContains, Values: []string{value}}
}

// ContainsAny is the same match against several columns at once, which is what
// contracts/records.md's `?q=` is: one substring, over the name and the
// alternative name.
//
// It is one term rather than two because two terms are ANDed, and a search that
// required the word to appear in both columns would answer almost nothing —
// silently, and looking exactly like a list that has no matches.
func ContainsAny(value string, columns ...string) Condition {
	return Condition{Columns: columns, Op: OpContains, Values: []string{value}}
}

// GTE and LTE are a date range's two halves. `?from=2026-01-01&to=2026-03-01`
// is Query{Conditions: []Condition{GTE(col, from), LTE(col, to)}} — two
// conjuncts, each applied only when its half of the range was given.
func GTE(column, value string) Condition {
	return Condition{Columns: []string{column}, Op: OpGTE, Values: []string{value}}
}

func LTE(column, value string) Condition {
	return Condition{Columns: []string{column}, Op: OpLTE, Values: []string{value}}
}

// AnyOf is `?tags=a,b&match=any`: the column's relation carries at least one
// of the given ids.
func AnyOf(column string, ids ...string) Condition {
	return Condition{Columns: []string{column}, Op: OpAnyOf, Values: ids}
}

// AllOf is `?tags=a,b&match=all`: the column's relation carries every one of
// the given ids.
func AllOf(column string, ids ...string) Condition {
	return Condition{Columns: []string{column}, Op: OpAllOf, Values: ids}
}

// Query is a list request, expressed in columns rather than in a filter string.
type Query struct {
	Conditions []Condition
	Sort       []domain.SortKey

	// After is the keyset boundary — the last row of the previous page. Its
	// ordering must be the one being paged; a boundary from another ordering
	// names a row that is somewhere else entirely in this sequence.
	After Cursor

	// Limit is clamped to nothing: zero means the published default, and
	// anything above MaxLimit is refused rather than quietly reduced.
	Limit int
}

// Built is the query as SQL: one expression, the ordering terms, and the limit.
type Built struct {
	// Where is nil when there is nothing to narrow by.
	Where   dbx.Expression
	OrderBy []string
	Limit   int

	// Sort is the ordering this was built for, which is what a cursor minted
	// from its last row must be bound to.
	Sort []domain.SortKey
}

// Apply puts the three onto a PocketBase record query.
func (b Built) Apply(query *dbx.SelectQuery) *dbx.SelectQuery {
	if b.Where != nil {
		query = query.AndWhere(b.Where)
	}

	return query.OrderBy(b.OrderBy...).Limit(int64(b.Limit))
}

// Column is one column a query may name.
type Column struct {
	Name string

	// Expr is the SQL this column is compared and ordered by. It is the
	// index's expression, not the bare column, wherever the two differ:
	// idx_medications_owner_name is built on LOWER(name), so ordering by
	// anything else is a filesort over the whole owner's rows.
	Expr string

	// Value reads the same value out of a record, and it is the same value or
	// keyset paging is silently wrong. LOWER() in SQLite folds ASCII and
	// nothing else; strings.ToLower folds the whole of Unicode, so the obvious
	// Go twin of LOWER(name) is the wrong one. filter_test.go asserts the two
	// agree, column by column, against a real database.
	Value func(record *core.Record) string

	// FilterOnly keeps the column out of every ordering and out of every
	// keyset boundary, while leaving it narrowable.
	//
	// The distinction is not tidiness. A sort column's value *is* the boundary
	// the next page is asked for, and a boundary travels in a query string —
	// through the browser's history, the Referer header and every reverse
	// proxy's access log. The cursor is authenticated encryption rather than a
	// signature precisely so a drug name cannot make that journey (research
	// D-29), and a column that may be ordered by is a second route to the same
	// disclosure. So the answer to "may this be ordered by" is declared here
	// rather than left to whoever writes the next resource's allowlist.
	FilterOnly bool

	// Searchable admits the column into a term that spans several columns at
	// once — the disjunction ContainsAny builds, and nothing else.
	//
	// This is the gate on the one shape in this package that is not a
	// conjunction. Every term is ANDed, and the owner scope is one of those
	// terms: that is what keeps one account's medications away from another's.
	// A term that is itself an OR is the only thing that can swallow that
	// predicate — widen the group by one column and the scope becomes
	// optional, with nothing else in the system objecting. An account
	// reference is not free text and is not searchable, so widening the group
	// that way is refused rather than reviewed.
	Searchable bool

	// AbsentLast orders a row that has no value for this column after every
	// row that has one, under both directions.
	//
	// contracts/records.md states it rather than leaving it to the database
	// precisely because SQLite's answer differs between the two directions: an
	// unset date column holds the empty string, which sorts before every real
	// date ascending and after every real one descending. A person whose
	// medication has no recorded start date should not find it at the top of
	// "earliest started" any more than at the top of "most recently started".
	AbsentLast bool
}

// absentFlag is the ordering prefix an AbsentLast column carries ascending: a
// present value sorts under "0" and an absent one under "1", so the absent rows
// land after every present one and the present ones keep their own order behind
// the shared prefix. It is a lexicographic composition of two ordering terms
// into one, which is what lets the keyset predicate stay one comparison per sort
// key rather than growing a hidden second term the cursor would have to carry.
const (
	absentFlagPresent = "0"
	absentFlagAbsent  = "1"
)

// sortExpr is the SQL this column is ordered and keyset-compared by in one
// direction.
//
// Descending is the bare expression, and that is not an oversight: the empty
// string is already smaller than every real value, so descending puts it last
// on its own — and the bare column is what idx_medications_owner_start is built
// on, so the default ordering still reads straight off the index. Only the
// ascending half needs the prefix, and only for a column that declared one.
func (c Column) sortExpr(desc bool) string {
	if !c.AbsentLast || desc {
		return c.Expr
	}

	return "(CASE WHEN " + c.Expr + " = '' THEN '" + absentFlagAbsent +
		"' ELSE '" + absentFlagPresent + "' END) || " + c.Expr
}

// sortValue is the Go twin of sortExpr, and it is the value a keyset boundary
// on this column carries. The two are computed from the same branch on purpose:
// a boundary read one way and compared another lands in the wrong place in the
// sequence and says nothing about it.
func (c Column) sortValue(record *core.Record, desc bool) string {
	value := c.Value(record)

	if !c.AbsentLast || desc {
		return value
	}

	if value == "" {
		return absentFlagAbsent
	}

	return absentFlagPresent + value
}

// Schema is a resource's published query surface: the columns a request may
// name, and nothing else.
type Schema struct {
	collection string
	order      []string
	columns    map[string]Column
}

// NewSchema declares one. The id column is added automatically because it is
// the keyset tiebreaker and every ordering ends in it.
func NewSchema(collection string, columns ...Column) Schema {
	schema := Schema{
		collection: collection,
		order:      make([]string, 0, len(columns)+1),
		columns:    make(map[string]Column, len(columns)+1),
	}

	schema.add(Column{Name: fieldID, Value: func(record *core.Record) string { return record.Id }})

	for _, column := range columns {
		schema.add(column)
	}

	return schema
}

func (s *Schema) add(column Column) {
	if column.Expr == "" {
		column.Expr = quoteColumn(column.Name)
	}

	if column.Value == nil {
		name := column.Name
		column.Value = func(record *core.Record) string { return record.GetString(name) }
	}

	if _, declared := s.columns[column.Name]; !declared {
		s.order = append(s.order, column.Name)
	}

	s.columns[column.Name] = column
}

func (s Schema) Collection() string { return s.collection }

// Columns is every declared column in declaration order, id first.
func (s Schema) Columns() []string {
	return append([]string(nil), s.order...)
}

func (s Schema) Column(name string) (Column, bool) {
	column, declared := s.columns[name]

	return column, declared
}

// MedicationSchema is the medication list's query surface.
//
// It lives beside the mapper rather than in the repository package because a
// column's SQL expression and its Go twin are the same knowledge the mapper
// holds, and splitting them puts the LOWER(name) pair in two files that have to
// be changed together.
//
// The absentees are the decision. dosage, frequency, indication, side_effects
// and notes are free text a person wrote about their own health; nothing in
// FR-022 narrows or orders by them, and an allowlist that lists every column is
// not one. alternative_name is here and they are not because contracts/records.md
// defines `?q=` as a substring over the name *and* the alternative name: a
// person who recorded the brand name and searched for the generic one is the
// case that column exists for.
func MedicationSchema() Schema {
	return NewSchema(kind.Medication.Collection(),
		Column{Name: medicationFieldPatient},
		Column{
			Name:       medicationFieldName,
			Expr:       "LOWER(" + quoteColumn(medicationFieldName) + ")",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(medicationFieldName))
			},
		},
		// Narrowable and never orderable. FR-022 searches it; nothing orders
		// by it, and FilterOnly is what stops the next person from adding it to
		// an ordering allowlist and putting a drug's other name into a cursor.
		Column{
			Name:       medicationFieldAlternativeName,
			Expr:       "LOWER(" + quoteColumn(medicationFieldAlternativeName) + ")",
			Searchable: true,
			FilterOnly: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(medicationFieldAlternativeName))
			},
		},
		Column{Name: medicationFieldType},
		Column{Name: medicationFieldRoute},
		Column{Name: medicationFieldStatus},
		Column{Name: medicationFieldStartedOn, AbsentLast: true},
		Column{Name: medicationFieldEndedOn},
		Column{Name: fieldCreated},
		Column{Name: fieldUpdated},
	)
}

// PatientsSchema is the patient list's query surface (research D-29): last
// name, first name and id, matching web.PatientsSort's published ordering,
// plus first and last name searchable for `?q=` (contracts/patients.md).
func PatientsSchema() Schema {
	return NewSchema(PatientCollection,
		Column{Name: PatientOwner},
		Column{
			Name:       patientFieldLastName,
			Expr:       "LOWER(" + quoteColumn(patientFieldLastName) + ")",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(patientFieldLastName))
			},
		},
		Column{
			Name:       patientFieldFirstName,
			Expr:       "LOWER(" + quoteColumn(patientFieldFirstName) + ")",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(patientFieldFirstName))
			},
		},
	)
}

// AccountSchema is the account collection's query surface, and it publishes
// exactly one narrowable column: the address.
//
// Not the display name, not the role, not the disabled instant. There is no
// account list in MediKube and no phase in this spec adds one — every account
// operation reaches the caller's own account and no other (FR-032) — so a
// schema that admitted a second column would be a query surface for a listing
// nobody asked for, over a table of people who have medical records here.
//
// The address is FilterOnly, which keeps it out of every ordering and therefore
// out of every keyset boundary. A boundary travels in a query string, through
// the browser's history, the Referer header and every reverse proxy's access
// log; an email address is the last value that should make that journey
// (research D-29).
//
// Its expression is LOWER(email) and not the bare column, because that is what
// idx_users_email_lower is built on: comparing anything else both misses the
// index and answers a different question from the one the unique constraint
// asks (FR-003).
func AccountSchema() Schema {
	return NewSchema(authCollection,
		Column{
			Name:       userFieldEmail,
			Expr:       "LOWER(" + quoteColumn(userFieldEmail) + ")",
			FilterOnly: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(userFieldEmail))
			},
		},
	)
}

// SameAddress is FR-003's comparison: two addresses differing only in letter
// case are one address.
//
// The fold is here rather than at the call site because it is the other half of
// the column's LOWER() expression, and the two have to agree byte for byte or
// the query silently stops matching the index it was written for. SQLite's
// LOWER() folds ASCII and nothing else; strings.ToLower folds the whole of
// Unicode, so the obvious Go twin is the wrong one.
func SameAddress(email string) Condition {
	return Equal(userFieldEmail, asciiLower(email))
}

// The medication columns a repository names when it builds a Query.
//
// Exported because internal/store/medication assembles the query and this
// package answers it, and a repository that spelled a column by hand would drift
// from the schema with nothing to notice: AssertMappedFields checks this
// package's names against the database, not a caller's, and a misspelled column
// in a Query is refused per request at runtime rather than at boot.
//
// The three the service already publishes as its sort vocabulary —
// medication.FieldName, FieldStartedOn, FieldUpdated — are deliberately not
// re-exported here. A sort field arrives from the service already spelled and is
// resolved against this schema; a second spelling of the same word is the drift
// this constant list exists to prevent.
const (
	ColumnID = fieldID

	MedicationPatient         = medicationFieldPatient
	MedicationPractitioner    = medicationFieldPractitioner
	MedicationPharmacy        = medicationFieldPharmacy
	MedicationName            = medicationFieldName
	MedicationAlternativeName = medicationFieldAlternativeName
	MedicationStatus          = medicationFieldStatus
)

// Build turns the query into SQL.
func (s Schema) Build(query Query) (Built, error) {
	limit, err := boundedLimit(query.Limit)
	if err != nil {
		return Built{}, err
	}

	binder := &paramBinder{params: dbx.Params{}}

	fragments := make([]string, 0, len(query.Conditions)+1)

	for _, condition := range query.Conditions {
		columns, resolveErr := s.resolveColumns(condition.Columns)
		if resolveErr != nil {
			return Built{}, resolveErr
		}

		fragment, conditionErr := renderCondition(columns, condition, binder)
		if conditionErr != nil {
			return Built{}, conditionErr
		}

		fragments = append(fragments, "("+fragment+")")
	}

	sortColumns, err := s.resolveSort(query.Sort)
	if err != nil {
		return Built{}, err
	}

	if !query.After.IsZero() {
		if !sameSort(query.After.Sort, query.Sort) {
			return Built{}, fmt.Errorf(
				"%w: the boundary was issued for a different ordering than the one being paged", ErrInvalidQuery)
		}

		if err := query.After.validate(); err != nil {
			return Built{}, err
		}

		fragments = append(fragments, "("+s.renderKeyset(sortColumns, query.After, binder)+")")
	}

	built := Built{
		OrderBy: orderTerms(sortColumns, query.Sort),
		Limit:   limit,
		Sort:    query.Sort,
	}

	if len(fragments) > 0 {
		built.Where = dbx.NewExp(strings.Join(fragments, " AND "), binder.params)
	}

	return built, nil
}

// Boundary mints the cursor for a page's last row. It reads each sort value
// through the column's own Value function, so the boundary is the value the
// ordering actually compared and not a second reading of the same column.
func (s Schema) Boundary(record *core.Record, sortKeys []domain.SortKey) (Cursor, error) {
	if record == nil || record.Id == "" {
		return Cursor{}, fmt.Errorf("%w: a boundary needs a row", ErrInvalidQuery)
	}

	cursor := Cursor{Sort: sortKeys, ID: record.Id}

	if len(sortKeys) > 0 {
		cursor.Values = make([]string, 0, len(sortKeys))
	}

	for _, key := range sortKeys {
		column, ok := s.orderable(key.Field)
		if !ok {
			return Cursor{}, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, key.Field)
		}

		cursor.Values = append(cursor.Values, column.sortValue(record, key.Desc))
	}

	return cursor, nil
}

// resolveColumns is one condition's columns, in the order it named them. An
// undeclared column is refused before anything is bound, and a condition that
// names none at all is refused outright: it would render as no fragment, which
// is a narrowing term that narrows nothing and a list that quietly holds
// somebody else's rows.
func (s Schema) resolveColumns(names []string) ([]Column, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: a condition that names no column narrows nothing", ErrInvalidQuery)
	}

	columns := make([]Column, 0, len(names))

	for _, name := range names {
		column, declared := s.columns[name]
		if !declared {
			return nil, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, name)
		}

		columns = append(columns, column)
	}

	return columns, nil
}

// resolveSort is the ordering, and a column declared FilterOnly is answered
// exactly as one the schema never had. The refusal is deliberately the same
// error: a distinct one would tell a caller that the column exists and only the
// ordering is closed, which is a map of the surface they were not given.
func (s Schema) resolveSort(sortKeys []domain.SortKey) ([]Column, error) {
	columns := make([]Column, 0, len(sortKeys))

	for _, key := range sortKeys {
		column, ok := s.orderable(key.Field)
		if !ok {
			return nil, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, key.Field)
		}

		columns = append(columns, column)
	}

	return columns, nil
}

// orderable is the one lookup an ordering and a boundary both go through, so a
// column that may not be ordered by cannot be reached by minting a cursor on it
// either.
func (s Schema) orderable(name string) (Column, bool) {
	column, declared := s.columns[name]
	if !declared || column.FilterOnly {
		return Column{}, false
	}

	return column, true
}

// renderKeyset is the lexicographic row comparison FR-023 needs:
//
//	e0 OP v0
//	OR (e0 = v0 AND e1 OP v1)
//	OR (e0 = v0 AND e1 = v1 AND id < vid)
//
// "everything after this row in this ordering" is a predicate a concurrent
// insert cannot change the meaning of. "skip the first 25 rows" is not — which
// is the whole of the difference between this and an offset.
//
// The id term is always last and always descending, matching the trailing
// `id DESC` on every index in data-model §2.
func (s Schema) renderKeyset(sortColumns []Column, after Cursor, binder *paramBinder) string {
	exprs := make([]string, 0, len(sortColumns)+1)
	values := make([]string, 0, len(sortColumns)+1)
	comparisons := make([]string, 0, len(sortColumns)+1)

	for i, column := range sortColumns {
		exprs = append(exprs, column.sortExpr(after.Sort[i].Desc))
		values = append(values, binder.bind(after.Values[i]))
		comparisons = append(comparisons, comparisonFor(after.Sort[i].Desc))
	}

	idColumn := s.columns[fieldID]
	exprs = append(exprs, idColumn.Expr)
	values = append(values, binder.bind(after.ID))
	comparisons = append(comparisons, comparisonFor(true))

	disjuncts := make([]string, 0, len(exprs))

	for i := range exprs {
		terms := make([]string, 0, i+1)
		for j := range i {
			terms = append(terms, exprs[j]+" = "+values[j])
		}

		terms = append(terms, exprs[i]+" "+comparisons[i]+" "+values[i])

		if len(terms) == 1 {
			disjuncts = append(disjuncts, terms[0])
			continue
		}

		disjuncts = append(disjuncts, "("+strings.Join(terms, " AND ")+")")
	}

	return strings.Join(disjuncts, " OR ")
}

func orderTerms(sortColumns []Column, sortKeys []domain.SortKey) []string {
	terms := make([]string, 0, len(sortColumns)+1)

	for i, column := range sortColumns {
		terms = append(terms, column.sortExpr(sortKeys[i].Desc)+" "+directionFor(sortKeys[i].Desc))
	}

	return append(terms, quoteColumn(fieldID)+" DESC")
}

// renderCondition is one term. The values are bound once and the placeholders
// are reused across the columns, so a search over two columns binds one
// parameter rather than the same string twice.
func renderCondition(columns []Column, condition Condition, binder *paramBinder) (string, error) {
	switch condition.Op {
	case OpEqual, OpNotEqual, OpContains, OpGTE, OpLTE:
		if len(condition.Values) != 1 {
			return "", fmt.Errorf("%w: %s takes exactly one value, not %d",
				ErrInvalidQuery, condition.Op, len(condition.Values))
		}
	case OpOneOf, OpAnyOf, OpAllOf:
		if len(condition.Values) == 0 {
			return "", fmt.Errorf("%w: %s of nothing matches nothing, which is a query nobody meant to ask",
				ErrInvalidQuery, condition.Op)
		}
	default:
		return "", fmt.Errorf("%w: %q is not one of this package's operators", ErrInvalidQuery, condition.Op)
	}

	// The disjunction is admitted only over columns the resource declared
	// searchable. A single-column term is a conjunct like every other and needs
	// no permission; several columns is the one shape that can make another
	// term optional, and the owner scope is a term.
	if len(columns) > 1 {
		for _, column := range columns {
			if !column.Searchable {
				return "", fmt.Errorf(
					"%w: %s may not be one of several columns in a single term — a term that spans more than one is a disjunction, and the owner scope is a term",
					ErrInvalidQuery, column.Name)
			}
		}
	}

	placeholders := make([]string, 0, len(condition.Values))

	switch condition.Op {
	case OpContains:
		placeholders = append(placeholders, binder.bind("%"+escapeLike(condition.Values[0])+"%"))
	case OpAnyOf, OpAllOf:
		// A MaxSelect:0 relation is stored as a JSON array of ids in one text
		// column; membership is a LIKE against the id wrapped in its JSON
		// quoting, not IN, which tests scalar equality against the whole
		// column and would never match a multi-valued one.
		for _, value := range condition.Values {
			placeholders = append(placeholders, binder.bind(`%"`+escapeLike(value)+`"%`))
		}
	default:
		for _, value := range condition.Values {
			placeholders = append(placeholders, binder.bind(value))
		}
	}

	fragments := make([]string, 0, len(columns))

	for _, column := range columns {
		fragment, err := renderComparison(column, condition.Op, placeholders)
		if err != nil {
			return "", err
		}

		fragments = append(fragments, fragment)
	}

	// No parentheses of its own: Build wraps every term in one, so adding a
	// second pair here would only be noise in the assertions that read the SQL.
	return strings.Join(fragments, " OR "), nil
}

func renderComparison(column Column, op Operator, placeholders []string) (string, error) {
	switch op {
	case OpEqual:
		return column.Expr + " = " + placeholders[0], nil
	case OpNotEqual:
		return column.Expr + " != " + placeholders[0], nil
	case OpContains:
		return column.Expr + " LIKE " + placeholders[0] + ` ESCAPE '\'`, nil
	case OpOneOf:
		return column.Expr + " IN (" + strings.Join(placeholders, ", ") + ")", nil
	case OpGTE:
		return column.Expr + " >= " + placeholders[0], nil
	case OpLTE:
		return column.Expr + " <= " + placeholders[0], nil
	case OpAnyOf, OpAllOf:
		joiner := " OR "
		if op == OpAllOf {
			joiner = " AND "
		}

		parts := make([]string, 0, len(placeholders))
		for _, p := range placeholders {
			parts = append(parts, column.Expr+` LIKE `+p+` ESCAPE '\'`)
		}

		return "(" + strings.Join(parts, joiner) + ")", nil
	default:
		return "", fmt.Errorf("%w: %q is not one of this package's operators", ErrInvalidQuery, op)
	}
}

func boundedLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return DefaultLimit, nil
	case limit < 0 || limit > MaxLimit:
		return 0, fmt.Errorf("%w: a page is between 1 and %d entries, not %d", ErrInvalidQuery, MaxLimit, limit)
	default:
		return limit, nil
	}
}

// paramBinder numbers the bound parameters. The prefix is MediKube's own so a
// fragment can be composed with one PocketBase built without either overwriting
// the other's values — dbx numbers its own from the length of the params map
// (expression.go:204-207), which is not a name it owns.
type paramBinder struct {
	params dbx.Params
	next   int
}

func (b *paramBinder) bind(value string) string {
	name := "mk" + strconv.Itoa(b.next)
	b.next++
	b.params[name] = value

	return "{:" + name + "}"
}

// escapeLike neutralises the wildcards a person typed, so a name containing a
// per-cent sign is a search for that name and not a search for everything.
// The escape character goes first, or escaping the others would escape the
// escapes.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}

// asciiLower is SQLite's LOWER(), not Go's.
//
// SQLite's built-in LOWER folds the twenty-six ASCII letters and leaves every
// other code point alone; strings.ToLower folds the whole of Unicode. A Turkish
// dotted capital I is the shortest counterexample: Go turns it into two code
// points and SQLite leaves it as one. The keyset boundary has to be the value
// the index holds, so this is the folding that matters.
func asciiLower(value string) string {
	var out strings.Builder

	out.Grow(len(value))

	for i := range len(value) {
		b := value[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}

		out.WriteByte(b)
	}

	return out.String()
}

func quoteColumn(name string) string { return "[[" + name + "]]" }

func comparisonFor(desc bool) string {
	if desc {
		return "<"
	}

	return ">"
}

func directionFor(desc bool) string {
	if desc {
		return "DESC"
	}

	return "ASC"
}
