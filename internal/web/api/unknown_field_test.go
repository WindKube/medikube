package api_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// T311. web.DecodeBytes turns on json.RejectUnknownMembers(true) once, in one
// place — but "once, in one place" is only as good as every write DTO
// actually going through it. This walks internal/web's own *.go sources for
// every `web.Decode(e, &x)` call site, finds the declared type of x, and
// requires that type to be registered in unknownFieldFixtures below so it is
// actually exercised.
//
// The registry is the deliberate seam: a source walk cannot instantiate a
// type it only knows by name, so a new DTO wired to web.Decode without an
// entry here fails this test loudly (naming the type) rather than being
// silently skipped — which is the failure mode a purely dynamic reflect-only
// walk would have.

// unknownFieldFixtures pairs a write DTO with a JSON object of the shape it
// actually accepts. TestEveryWriteDTORejectsUnknownFields adds one member
// neither of them declares and asserts the decode refuses it.
var unknownFieldFixtures = map[string]any{
	"RegisterRequest": api.RegisterRequest{
		Email: "person@example.com", Name: "A Person", Password: "correct horse battery staple",
	},
	"LoginRequest": api.LoginRequest{
		Email: "person@example.com", Password: "correct horse battery staple",
	},
	"PasswordResetRequest": api.PasswordResetRequest{Email: "person@example.com"},
	"PasswordResetConfirm": api.PasswordResetConfirm{
		Token: "tok", Password: "correct horse battery staple", PasswordConfirm: "correct horse battery staple",
	},
	"EmailVerificationConfirm": api.EmailVerificationConfirm{Token: "tok"},
	"MePatch":                  api.MePatch{},
	"ChangePasswordRequest": api.ChangePasswordRequest{
		CurrentPassword: "old", NewPassword: "correct horse battery staple",
	},
	"DeleteAccountRequest": api.DeleteAccountRequest{
		Password: "old", Confirmation: "DELETE",
	},
	"PatientCreate": api.PatientCreate{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: "1988-04-12",
	},
	"PatientPatch": api.PatientPatch{
		FirstName: &patientPatchFirstNameFixture,
	},
	"PractitionerCreate": api.PractitionerCreate{Name: "Dr. Fixture"},
	"PractitionerPatch":  api.PractitionerPatch{},
	"FacilityCreate":     api.FacilityCreate{Kind: "practice", Name: "Fixture Facility"},
	"FacilityPatch":      api.FacilityPatch{},
	"ActivePatientBody":  api.ActivePatientBody{Patient: &activePatientFixture},
	"TagCreate":          api.TagCreate{Name: "cardiology", Color: "#aa3311"},
	"TagPatch":           api.TagPatch{},
	"CourseMedicationPut": api.CourseMedicationPut{
		Dosage: &courseMedicationDosageFixture,
	},
}

var courseMedicationDosageFixture = "3mg"

var activePatientFixture = "pat0000000000001"

var patientPatchFirstNameFixture = "Amara"

func TestEveryWriteDTORejectsUnknownFields(t *testing.T) {
	t.Parallel()

	found := decodedDTOTypeNames(t)
	require.NotEmpty(t, found, "the source walk found no web.Decode(e, &x) call site — the parser is broken, not the handlers")

	for _, name := range found {
		fixture, registered := unknownFieldFixtures[name]
		if !assert.Truef(t, registered,
			"%s is decoded via web.Decode but has no fixture in unknownFieldFixtures — add one so this gate actually exercises it", name) {
			continue
		}

		t.Run(name, func(t *testing.T) {
			assertRejectsUnknownField(t, fixture)
		})
	}

	// And the reverse: a fixture for a type nothing decodes any more is a
	// registry that has drifted from the handlers rather than a stricter one.
	for name := range unknownFieldFixtures {
		assert.Containsf(t, found, name,
			"unknownFieldFixtures registers %s, but no web.Decode call site in internal/web decodes it any more", name)
	}
}

func assertRejectsUnknownField(t *testing.T, fixture any) {
	t.Helper()

	valid, err := json.Marshal(fixture)
	require.NoError(t, err)

	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(valid, &members))

	members["zzz_a_field_no_dto_declares"] = json.RawMessage(`"anything"`)

	tainted, err := json.Marshal(members)
	require.NoError(t, err)

	target := reflect.New(reflect.TypeOf(fixture)).Interface()

	decodeErr := web.DecodeBytes(tainted, target)
	require.Error(t, decodeErr, "an unknown member was accepted")

	var invalid *domain.ValidationError
	require.ErrorAs(t, decodeErr, &invalid, "the refusal is not a domain.ValidationError")

	var sawUnknownField bool
	for _, field := range invalid.Fields {
		if field.Code == domain.CodeUnknownField {
			sawUnknownField = true
		}
	}
	assert.True(t, sawUnknownField, "the refusal did not carry domain.CodeUnknownField")
}

// decodedDTOTypeNames walks internal/web's own *.go sources (not tests, not
// subpackages other than what web.Decode is called from) for `web.Decode(e,
// &IDENT)` call sites and resolves IDENT to the type name declared for it by
// a preceding `var IDENT Type` in the same function.
func decodedDTOTypeNames(t *testing.T) []string {
	t.Helper()

	root := repoRoot(t)
	dir := filepath.Join(root, "internal/web")

	seen := map[string]bool{}
	var names []string

	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, parseErr, "parsing %s", path)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			declared := map[string]string{}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if assign := varDecl(n); assign != nil {
					declared[assign.name] = assign.typeName
				}

				if callArg := decodeCallArg(n); callArg != "" {
					if typeName, ok := declared[callArg]; ok && !seen[typeName] {
						seen[typeName] = true
						names = append(names, typeName)
					}
				}

				return true
			})
		}

		return nil
	})
	require.NoError(t, err)

	return names
}

type varSpec struct {
	name     string
	typeName string
}

// varDecl matches `var ident Type` inside a function body.
func varDecl(n ast.Node) *varSpec {
	decl, ok := n.(*ast.DeclStmt)
	if !ok {
		return nil
	}

	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return nil
	}

	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != 1 {
			continue
		}

		ident, ok := value.Type.(*ast.Ident)
		if !ok {
			continue
		}

		return &varSpec{name: value.Names[0].Name, typeName: ident.Name}
	}

	return nil
}

// decodeCallArg matches `web.Decode(e, &ident)` and returns ident's name.
func decodeCallArg(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return ""
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Decode" {
		return ""
	}

	pkg, isIdent := sel.X.(*ast.Ident)
	if !isIdent || pkg.Name != "web" {
		return ""
	}

	unary, ok := call.Args[1].(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return ""
	}

	ident, ok := unary.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return ident.Name
}
