package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T002. Constitution Principle II's open/closed clause: internal/records
// dispatches every kind through its registry, and a `switch` or an if-chain
// over a kind.Kind-typed expression anywhere else is the mechanism that clause
// forbids — adding the fourteenth kind must change no code that already
// exists, and a switch statement is exactly the code a new case has to be
// added to.
//
// Only internal/records/ (the dispatcher research D-05 names) and
// internal/domain/kind/ (the table itself, whose own tests walk its constants)
// may do this.
var kindSwitchExempt = []string{
	"internal/records/",
	"internal/domain/kind/",
}

func TestNoSwitchOrIfChainSwitchesOnAKindKindTypedExpression(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()

	var offences []string

	walkRepo(t, root, func(rel string, _ fs.DirEntry) {
		if filepath.Ext(rel) != ".go" {
			return
		}

		for _, prefix := range kindSwitchExempt {
			if strings.HasPrefix(rel, prefix) {
				return
			}
		}

		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
		require.NoErrorf(t, err, "parsing %s", rel)

		for _, offence := range findKindSwitches(fset, file) {
			offences = append(offences, rel+":"+offence)
		}
	})

	assert.Empty(t, offences)
}

// findKindSwitches walks every function body in a file and reports a switch
// or an if/else-if chain whose condition compares a kind.Kind-typed
// identifier.
//
// This is syntactic, not type-checked: it tracks identifiers declared with the
// literal type expression `kind.Kind` — a function parameter, a var, or a
// named result — within the function they are declared in. That is exactly
// how every kind.Kind value in this codebase is spelled (research D-05: there
// is one table, and everything else takes a Kind as a parameter or holds one
// in a local), so it catches the violation this gate exists for without the
// cost of loading go/types across the whole module.
func findKindSwitches(fset *token.FileSet, file *ast.File) []string {
	var offences []string

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		kindTyped := map[string]bool{}
		collectKindTypedNames(fn.Type.Params, kindTyped)
		collectKindTypedNames(fn.Type.Results, kindTyped)

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				if isKindKindType(node.Type) {
					for _, name := range node.Names {
						kindTyped[name.Name] = true
					}
				}
			case *ast.SwitchStmt:
				if node.Tag != nil && identIsKindTyped(node.Tag, kindTyped) {
					offences = append(offences, position(fset, node.Pos())+
						": a switch over a kind.Kind-typed expression — dispatch through internal/records instead")
				}
			case *ast.IfStmt:
				if isKindComparisonChain(node, kindTyped) {
					offences = append(offences, position(fset, node.Pos())+
						": an if-chain over a kind.Kind-typed expression — dispatch through internal/records instead")
				}
			}

			return true
		})
	}

	return offences
}

func collectKindTypedNames(fields *ast.FieldList, into map[string]bool) {
	if fields == nil {
		return
	}

	for _, field := range fields.List {
		if !isKindKindType(field.Type) {
			continue
		}

		for _, name := range field.Names {
			into[name.Name] = true
		}
	}
}

// isKindKindType is the literal type expression `kind.Kind`.
func isKindKindType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkg, ok := selector.X.(*ast.Ident)

	return ok && pkg.Name == "kind" && selector.Sel.Name == "Kind"
}

func identIsKindTyped(expr ast.Expr, kindTyped map[string]bool) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && kindTyped[ident.Name]
}

// isKindComparisonChain reports an if statement whose condition compares a
// kind.Kind-typed identifier for equality, and which chains into at least one
// `else if` — the shape a `switch` would otherwise have been written as.
func isKindComparisonChain(stmt *ast.IfStmt, kindTyped map[string]bool) bool {
	if !conditionComparesKindTyped(stmt.Cond, kindTyped) {
		return false
	}

	_, chains := stmt.Else.(*ast.IfStmt)

	return chains
}

func conditionComparesKindTyped(cond ast.Expr, kindTyped map[string]bool) bool {
	binary, ok := cond.(*ast.BinaryExpr)
	if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
		return false
	}

	return identIsKindTyped(binary.X, kindTyped) || identIsKindTyped(binary.Y, kindTyped)
}

func position(fset *token.FileSet, pos token.Pos) string {
	return strconv.Itoa(fset.Position(pos).Line)
}

// The mechanical proof the scanner recognises what it exists to catch —
// otherwise TestNoSwitchOrIfChainSwitchesOnAKindKindTypedExpression could be
// green because nothing in the repository trips it, or because nothing could.
func TestTheKindSwitchScannerRecognisesWhatItLooksFor(t *testing.T) {
	t.Parallel()

	source := "package sample\n\n" +
		"import \"medikube/internal/domain/kind\"\n\n" +
		"func dispatch(k kind.Kind) string {\n" +
		"\tswitch k {\n" +
		"\tcase kind.Medication:\n" +
		"\t\treturn \"medication\"\n" +
		"\tdefault:\n" +
		"\t\treturn \"other\"\n" +
		"\t}\n" +
		"}\n\n" +
		"func dispatchByIfChain(k kind.Kind) string {\n" +
		"\tif k == kind.Medication {\n" +
		"\t\treturn \"medication\"\n" +
		"\t} else if k == kind.Allergy {\n" +
		"\t\treturn \"allergy\"\n" +
		"\t}\n" +
		"\treturn \"other\"\n" +
		"}\n\n" +
		"func notAViolation(k kind.Kind) bool {\n" +
		"\treturn k == kind.Medication\n" +
		"}\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", source, 0)
	require.NoError(t, err)

	offences := findKindSwitches(fset, file)
	require.Len(t, offences, 2, "expected the switch and the if-chain, and nothing from the lone comparison")
}
