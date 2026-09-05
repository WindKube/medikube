package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every vocabulary under internal/domain is declared twice: a `const` block,
// and a package-level slice some forty lines below it that publishes the same
// values in the same order. Valid() reads the slice, the migration writes the
// slice into a SelectField, and the form offers the slice — so a constant that
// never reached the slice is a value the database refuses at write time, in
// production, on the first record that uses it.
//
// Nothing else in the suite can see that. audit/enums_test.go asserts Actions()
// against a hand-written list, which is the same hand-maintained list a second
// time: append a constant to the const block and to neither list and every test
// in the repository stays green, lint included. Eight such constants were
// planted across four packages and the suite did not move.
const enumTree = "internal/domain"

// The published slice for each vocabulary, keyed by the type it publishes. An
// entry is required rather than optional: a new enum type that nobody adds here
// is an offence below, because "this type has no slice" and "this type's slice
// is complete" produce the same green tick otherwise.
//
// kind.Kind's declaration is `registry []entry` and not `[]Kind` — a kind
// carries three spellings and the other two live in the same row (research
// D-05) — which is why the check is "the constant is named in the declaration"
// rather than "the slice's element type is the enum".
var enumSlices = map[string]string{
	"access.Permission":          "permissions",
	"audit.ActorKind":            "actorKinds",
	"audit.Action":               "actions",
	"audit.TargetKind":           "targetKinds",
	"clinical.MedicationType":    "medicationTypes",
	"clinical.MedicationRoute":   "medicationRoutes",
	"clinical.TherapyStatus":     "therapyStatuses",
	"identity.Role":              "roles",
	"identity.UnitSystem":        "unitSystems",
	"identity.DateFormat":        "dateFormats",
	"identity.Theme":             "themes",
	"kind.Kind":                  "registry",
	"person.Sex":                 "sexes",
	"person.BloodType":           "bloodTypes",
	"person.RelationshipToOwner": "relationshipsToOwner",
	"directory.FacilityKind":     "facilityKinds",
	"directory.Specialty":        "specialties",
}

func TestEveryDeclaredEnumConstantIsCarriedByItsPublishedSlice(t *testing.T) {
	t.Parallel()

	packages := parseEnumPackages(t)
	require.Greater(t, len(packages), 3,
		"the walk found almost no packages under %s; it is not looking where it thinks it is", enumTree)

	var (
		offences  []string
		constants int
		covered   = map[string]bool{}
	)

	for _, pkg := range packages {
		for _, typeName := range slices.Sorted(maps.Keys(pkg.constsByType)) {
			if !pkg.types[typeName] {
				// A constant of a type this package does not declare —
				// `const x time.Duration = 5` — is not a vocabulary.
				continue
			}

			declared := pkg.constsByType[typeName]
			qualified := pkg.name + "." + typeName

			constants += len(declared)
			covered[qualified] = true

			table, named := enumSlices[qualified]
			if !named {
				offences = append(offences, declared[0].position+": "+qualified+
					" declares constants and names no published slice — add it to enumSlices, "+
					"or the values ship with nothing asserting they are offered")

				continue
			}

			published, exists := pkg.varIdents[table]
			if !exists {
				offences = append(offences, qualified+" is published by "+table+
					", which is not a package-level declaration in "+pkg.name+" — enumSlices is stale")

				continue
			}

			for _, constant := range declared {
				if published[constant.name] {
					continue
				}

				offences = append(offences, constant.position+": "+qualified+" declares "+constant.name+
					" and "+table+" does not carry it — Valid() refuses it and the SelectField refuses it, "+
					"so the value fails at write time in production")
			}
		}
	}

	for _, qualified := range slices.Sorted(maps.Keys(enumSlices)) {
		if !covered[qualified] {
			offences = append(offences, qualified+
				" is in enumSlices and declares no constants — the entry names a type that has moved or gone")
		}
	}

	require.Greater(t, constants, 50,
		"the walk found almost no enum constants; it is not looking where it thinks it is")

	sort.Strings(offences)
	assert.Empty(t, offences)
}

// enumConstant is one declared value and where it was declared, so a failure
// sends the reader to the line rather than to the package.
type enumConstant struct {
	name     string
	position string
}

// enumPackage is one directory's declarations: the types it declares, the
// typed constants it declares, and — for every package-level var — the
// identifiers its initialiser names.
type enumPackage struct {
	name         string
	types        map[string]bool
	constsByType map[string][]enumConstant
	varIdents    map[string]map[string]bool
}

func parseEnumPackages(t *testing.T) []*enumPackage {
	t.Helper()

	root := repoRoot(t)
	fset := token.NewFileSet()

	byDir := map[string]*enumPackage{}
	dirOfPackage := map[string]string{}

	walkRepo(t, root, func(rel string, _ fs.DirEntry) {
		// Test files are excluded: a constant declared in one is a fixture, and
		// requiring it to be published would be requiring the fixture to ship.
		if !isUnder(rel, enumTree) || filepath.Ext(rel) != ".go" || strings.HasSuffix(rel, "_test.go") {
			return
		}

		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "parsing %s", rel)

		dir := filepath.Dir(rel)

		pkg, seen := byDir[dir]
		if !seen {
			pkg = &enumPackage{
				name:         file.Name.Name,
				types:        map[string]bool{},
				constsByType: map[string][]enumConstant{},
				varIdents:    map[string]map[string]bool{},
			}
			byDir[dir] = pkg

			// The table is keyed by package name, so two directories sharing one
			// would make an entry ambiguous rather than wrong-looking.
			other, clash := dirOfPackage[pkg.name]
			require.Falsef(t, clash, "%s and %s are both package %s", dir, other, pkg.name)
			dirOfPackage[pkg.name] = dir
		}

		collectEnumDeclarations(fset, file, pkg)
	})

	packages := make([]*enumPackage, 0, len(byDir))
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		packages = append(packages, byDir[dir])
	}

	return packages
}

func collectEnumDeclarations(fset *token.FileSet, file *ast.File, pkg *enumPackage) {
	for _, decl := range file.Decls {
		gen, isGeneric := decl.(*ast.GenDecl)
		if !isGeneric {
			continue
		}

		switch gen.Tok {
		case token.TYPE:
			for _, spec := range gen.Specs {
				if typed, isType := spec.(*ast.TypeSpec); isType {
					pkg.types[typed.Name.Name] = true
				}
			}
		case token.CONST:
			collectConstBlock(fset, gen, pkg)
		case token.VAR:
			collectVarBlock(gen, pkg)
		}
	}
}

// collectConstBlock carries the type down the block. A ConstSpec with no
// expression list repeats the preceding one *and its type*, which is how
// access.Permission's iota ladder declares three constants while naming
// Permission once — and a walk that read only the first would miss two of them.
func collectConstBlock(fset *token.FileSet, gen *ast.GenDecl, pkg *enumPackage) {
	repeated := ""

	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}

		switch {
		case value.Type != nil:
			repeated = localTypeName(value.Type)
		case len(value.Values) > 0:
			// A fresh expression list with no type ends the repetition.
			repeated = ""
		}

		if repeated == "" {
			continue
		}

		for _, name := range value.Names {
			if name.Name == "_" {
				continue
			}

			pkg.constsByType[repeated] = append(pkg.constsByType[repeated], enumConstant{
				name:     name.Name,
				position: fset.Position(name.Pos()).String(),
			})
		}
	}
}

func collectVarBlock(gen *ast.GenDecl, pkg *enumPackage) {
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}

		named := map[string]bool{}

		for _, expr := range value.Values {
			ast.Inspect(expr, func(node ast.Node) bool {
				if ident, isIdent := node.(*ast.Ident); isIdent {
					named[ident.Name] = true
				}

				return true
			})
		}

		for _, name := range value.Names {
			pkg.varIdents[name.Name] = named
		}
	}
}

// localTypeName is the name only when the type is declared in this package. A
// qualified type is somebody else's and carries no vocabulary of ours.
func localTypeName(expr ast.Expr) string {
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		return ident.Name
	}

	return ""
}
