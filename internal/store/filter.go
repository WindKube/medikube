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
)

// Condition is one narrowing term. Build them with the constructors below
// rather than by hand — the shape of Values depends on the operator, and the
// constructors are what make that unrepresentable at the call site.
type Condition struct {
	Column string
	Op     Operator
	Values []string
}

func Equal(column, value string) Condition {
	return Condition{Column: column, Op: OpEqual, Values: []string{value}}
}

func NotEqual(column, value string) Condition {
	return Condition{Column: column, Op: OpNotEqual, Values: []string{value}}
}

func OneOf(column string, values ...string) Condition {
	return Condition{Column: column, Op: OpOneOf, Values: values}
}

// Contains is FR-022's "text match against the name".
//
// SQLite's LIKE folds ASCII case and nothing else, so this matches
// case-insensitively for Latin text and exactly for everything else. That is a
// property of the database, not a decision made here, and it is why the name
// column sorts by LOWER() but matches by LIKE.
func Contains(column, value string) Condition {
	return Condition{Column: column, Op: OpContains, Values: []string{value}}
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
// The absentees are the decision. alternative_name, dosage, frequency,
// indication, side_effects and notes are free text a person wrote about their
// own health; nothing in FR-022 narrows or orders by them, and an allowlist
// that lists every column is not one.
func MedicationSchema() Schema {
	return NewSchema(kind.Medication.Collection(),
		Column{Name: medicationFieldOwner},
		Column{
			Name: medicationFieldName,
			Expr: "LOWER(" + quoteColumn(medicationFieldName) + ")",
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(medicationFieldName))
			},
		},
		Column{Name: medicationFieldType},
		Column{Name: medicationFieldRoute},
		Column{Name: medicationFieldStatus},
		Column{Name: medicationFieldStartedOn},
		Column{Name: medicationFieldEndedOn},
		Column{Name: fieldCreated},
		Column{Name: fieldUpdated},
	)
}

// Build turns the query into SQL.
func (s Schema) Build(query Query) (Built, error) {
	limit, err := boundedLimit(query.Limit)
	if err != nil {
		return Built{}, err
	}

	binder := &paramBinder{params: dbx.Params{}}

	fragments := make([]string, 0, len(query.Conditions)+1)

	for _, condition := range query.Conditions {
		column, declared := s.columns[condition.Column]
		if !declared {
			return Built{}, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, condition.Column)
		}

		fragment, conditionErr := renderCondition(column, condition, binder)
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
		column, declared := s.columns[key.Field]
		if !declared {
			return Cursor{}, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, key.Field)
		}

		cursor.Values = append(cursor.Values, column.Value(record))
	}

	return cursor, nil
}

func (s Schema) resolveSort(sortKeys []domain.SortKey) ([]Column, error) {
	columns := make([]Column, 0, len(sortKeys))

	for _, key := range sortKeys {
		column, declared := s.columns[key.Field]
		if !declared {
			return nil, fmt.Errorf("%w: %s.%s", ErrUnknownColumn, s.collection, key.Field)
		}

		columns = append(columns, column)
	}

	return columns, nil
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
		exprs = append(exprs, column.Expr)
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
		terms = append(terms, column.Expr+" "+directionFor(sortKeys[i].Desc))
	}

	return append(terms, quoteColumn(fieldID)+" DESC")
}

func renderCondition(column Column, condition Condition, binder *paramBinder) (string, error) {
	switch condition.Op {
	case OpEqual, OpNotEqual, OpContains:
		if len(condition.Values) != 1 {
			return "", fmt.Errorf("%w: %s takes exactly one value, not %d",
				ErrInvalidQuery, condition.Op, len(condition.Values))
		}
	case OpOneOf:
		if len(condition.Values) == 0 {
			return "", fmt.Errorf("%w: %s of nothing matches nothing, which is a query nobody meant to ask",
				ErrInvalidQuery, condition.Op)
		}
	default:
		return "", fmt.Errorf("%w: %q is not one of this package's operators", ErrInvalidQuery, condition.Op)
	}

	switch condition.Op {
	case OpEqual:
		return column.Expr + " = " + binder.bind(condition.Values[0]), nil
	case OpNotEqual:
		return column.Expr + " != " + binder.bind(condition.Values[0]), nil
	case OpContains:
		return column.Expr + " LIKE " + binder.bind("%"+escapeLike(condition.Values[0])+"%") + ` ESCAPE '\'`, nil
	case OpOneOf:
		placeholders := make([]string, 0, len(condition.Values))
		for _, value := range condition.Values {
			placeholders = append(placeholders, binder.bind(value))
		}

		return column.Expr + " IN (" + strings.Join(placeholders, ", ") + ")", nil
	default:
		return "", fmt.Errorf("%w: %q is not one of this package's operators", ErrInvalidQuery, condition.Op)
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
