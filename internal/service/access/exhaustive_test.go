package access_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T225, FR-032. Every service method that touches a record reaches the
// checkpoint, asserted against the source rather than against a curated list of
// methods somebody remembered to add.
//
// The reason this is a source walk and not a table of calls is the failure it
// exists to catch: a method that FORGETS. A table only ever contains the
// methods somebody thought of, and a forgotten method is by definition not one
// of them — which is how internal/web/page's predecessor let three finished
// pages ship answering 501. Here the population is computed, the exemptions are
// written down with their reasons, and both counts are guarded, so the day this
// stops finding methods it fails instead of passing.
//
// The rule has two clauses, and a method satisfies it by either:
//
//   - reaching the checkpoint — a call to a method of the package's own
//     authorization port, the interface it declares that answers with an
//     access.Grant; or
//   - handing the actor onward — a call that passes the actor to a method of a
//     seam the package declares whose contract itself takes an access.Actor.
//     That is what internal/records' generic handler does: it decides nothing
//     and dispatches to the registered kind service, which is in this
//     population and does.
//
// Both clauses follow calls THROUGH the package's own helpers, because
// `s.authorizeRecord(...)` is how every medication method actually reaches the
// checkpoint and a walk that only looked at direct calls would demand the
// helper be inlined five times.

// The trees walked. internal/service is where the use cases live and
// internal/records is the one generic handler every record request passes
// through; nothing else takes an access.Actor and reaches a record.
var checkpointRoots = []string{"internal/service", "internal/records"}

const (
	// The actor's package and type. A method that takes one of these is a
	// method acting on somebody's behalf, which is the whole population.
	actorPackage = "medikube/internal/domain/access"
	actorType    = "Actor"

	// Grant is what an authorization port answers with, and it is the only
	// thing in MediKube that returns one. That is how a checkpoint port is
	// recognised here rather than by being called "Authorizer": a package could
	// rename the interface and this would still find it, and a package that
	// declared no such port at all has no checkpoint to reach — which is the
	// case this gate is loudest about.
	grantType = "Grant"

	// The concrete checkpoint, for a package that ever holds it directly
	// instead of behind its own port.
	checkpointPackage = "medikube/internal/service/access"
	checkpointType    = "Authorizer"
)

// Packages inside the walked trees that the rule does not apply to. Keyed by
// the exact package directory and never by a prefix: `internal/service/` as a
// prefix would have exempted every service package there will ever be, which is
// the bug internal/store/filter_test.go:697 records against a prefix that
// silently covered four packages more than its comment claimed.
var checkpointPackagesOutsideTheRule = map[string]string{
	"internal/service/access": "the checkpoint itself. It is what the rule points at, so requiring it to call something to authorize would be requiring a second checkpoint — which is the thing this story exists to prevent (T239)",

	"internal/records/recordstest":                   "the record family's hand-written fakes (T108). A fake kind service exists to be driven without a database or a checkpoint; a fake that authorized would be testing itself",
	"internal/service/medication/medicationtest":     "the medication contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, and its repository fake answers from a map",
	"internal/service/immunization/immunizationtest": "the immunization contract suite and its in-memory fake, for the same reason",
	"internal/service/injury/injurytest":             "the injury contract suite and its in-memory fake, for the same reason",
	"internal/service/identity/identitytest":         "the account contract suite and its in-memory fake, for the same reason",
	"internal/service/patient/patienttest":           "the patient contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/equipment/equipmenttest":       "the equipment contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/insurance/insurancetest":       "the insurance contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/symptom/symptomtest":           "the symptom contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/vitals/vitalstest":             "the vitals contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/encounter/encountertest":       "the encounter contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, and its repository fake answers from a map",
	"internal/service/procedure/proceduretest":       "the procedure contract suite and its in-memory fake, for the same reason",
	"internal/service/treatment/treatmenttest":       "the treatment contract suite and its in-memory fake, for the same reason",

	"internal/service/allergy/allergytest":                   "the allergy contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/condition/conditiontest":               "the condition contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/emergencycontact/emergencycontacttest": "the emergency-contact contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
	"internal/service/familymember/familymembertest":         "the family member contract suite and its in-memory fake. Its Authorizer IS a stub checkpoint, for the same reason",
}

// exemption is one method the rule does not reach, and why.
//
// insteadReaches is what keeps the exemption from being a free pass: the method
// is excused from the RECORD checkpoint and is still asserted to reach the
// named refusal — a package-local function, method or port call it must pass
// through. An exemption carrying one is checked in both directions; an
// exemption with an empty one rests on the prose beside it and nothing more,
// which is a fact stated here rather than hidden.
type exemption struct {
	reason         string
	insteadReaches string
}

// The methods that take an actor and reach no record.
//
// Every one of them is in internal/service/identity, and that is a deliberate
// decision rather than an accident of what happened to fail:
//
// identity IS in this population. It is a service package, its methods take an
// access.Actor and they reach stored accounts, so leaving it out would have
// meant the gate never looked at eleven methods that touch a person's data. It
// declares no checkpoint port because there is nothing for a record checkpoint
// to decide: every one of these methods reaches EXACTLY ONE account, the
// actor's own, and there is no id parameter anywhere in the package for a
// caller to name another one in (contracts/account.md, and the type-level half
// of FR-032). A checkpoint call there would be a lookup of the actor's own id
// against the actor's own id.
//
// What replaces it is named per method below. The account operations reach
// identity.authorize, which refuses a caller with no MediKube account and
// refuses a superuser on the same terms this checkpoint does; the link
// operations reach Redeem, where the token is the authorization; the sign-in
// reaches Authenticate. Two are genuinely unauthorized by design and say so.
var checkpointExempt = map[string]exemption{
	"internal/service/identity.Service.Me": {
		reason:         "reads the actor's own account and no other. There is no id to check and no second account to reach",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.UpdateProfile": {
		reason:         "patches the actor's own account. FR-012 is enforced by Profile having no member for the role, the address or the disabled instant, not by a checkpoint",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.ChangePassword": {
		reason:         "changes the actor's own credential, and proves it is still them with the current password (FR-009)",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.DeleteAccount": {
		reason:         "deletes the actor's own account, on the password and the confirmation phrase (FR-013)",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.RequestVerification": {
		reason:         "mails the actor's own stored address. It takes no address precisely so that a signed-in caller cannot aim the mailer at a stranger (FR-075)",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.SignOut": {
		reason:         "ends the actor's own sessions. It reads no account, because an account too disabled to be read must still be able to sign out of (FR-007)",
		insteadReaches: "authorize",
	},
	"internal/service/identity.Service.ConfirmPasswordReset": {
		reason:         "the caller is not signed in: the recovery link is what names the account, and the token is the authorization (FR-074)",
		insteadReaches: "redeem",
	},
	"internal/service/identity.Service.ConfirmVerification": {
		reason:         "the same, for the address-confirmation link. The person following it may not be signed in on that device (FR-075)",
		insteadReaches: "redeem",
	},
	"internal/service/identity.Service.SignIn": {
		reason:         "there is no actor yet — this is what produces one. The credential is the authorization",
		insteadReaches: "Authenticate",
	},
	"internal/service/identity.Service.Register": {
		reason: "creates the account there is nothing to authorize against yet. Whether it may run at all is instance-wide configuration (s.registrationOpen, FR-002) and identical for every caller, which is why it is not an access decision. PROSE ONLY: nothing below proves this one",
	},
	"internal/service/identity.Service.RequestPasswordReset": {
		reason: "anonymous by requirement. It takes an address and no actor — the parameter is `_` — and answers identically whether the address has an account, which is the oracle FR-073 closes. PROSE ONLY: nothing below proves this one",
	},
	"internal/service/patient.Service.List": {
		reason:         "reads only the actor's own patients: the repository query is filtered by owner and there is no id parameter anywhere for a caller to name another account's row in (contracts/patients.md's unconditional owner scope). Get is where a single patient is authorized against the person (research D-05)",
		insteadReaches: "Authenticated",
	},
	"internal/service/patient.Service.Create": {
		reason:         "creates a new patient for the actor; owner is taken from the session and there is no existing row to authorize against yet (FR-002)",
		insteadReaches: "Authenticated",
	},

	// practitioner and facility each declare their own trivial Authorizer
	// implementation in their own package (ports.go / authorizer.go) rather
	// than importing internal/service/access's, because the directory has no
	// share and no level beyond ownership to resolve. Each one IS the
	// checkpoint the package's own service methods call, so requiring it to
	// call something to authorize would be requiring a second checkpoint —
	// the same reasoning internal/service/access's own package-level
	// exemption states above. PROSE ONLY: nothing below proves these four.
	"internal/service/practitioner.actorAuthorizer.Actor": {
		reason: "IS the practitioner checkpoint itself",
	},
	"internal/service/practitioner/practitionertest.Authorizer.Actor": {
		reason: "the practitioner contract suite's fake Authorizer, for the same reason",
	},
	"internal/service/facility.defaultAuthorizer.Actor": {
		reason: "IS the facility checkpoint itself",
	},
	"internal/service/facility/facilitytest.Authorizer.Actor": {
		reason: "the facility contract suite's fake Authorizer, for the same reason",
	},
	"internal/service/tag.actorAuthorizer.Actor": {
		reason: "IS the tag checkpoint itself",
	},
	"internal/service/tag/tagtest.Authorizer.Actor": {
		reason: "the tag contract suite's fake Authorizer, for the same reason",
	},
}

func TestEveryServiceMethodThatTouchesARecordReachesTheCheckpoint(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var (
		offences   []string
		packages   int
		population int
		checked    int
		holders    int
	)

	for _, dir := range servicePackages(t, root) {
		if _, outside := checkpointPackagesOutsideTheRule[dir]; outside {
			continue
		}

		packages++

		facts := analyse(t, root, dir)
		holders += len(facts.checkpointFields)

		for _, key := range facts.population() {
			population++

			if _, exempt := checkpointExempt[dir+"."+key]; exempt {
				continue
			}

			checked++

			if !facts.reachesTheCheckpoint(key) {
				offences = append(offences, dir+"."+key+
					" takes an access.Actor and never reaches the authorization checkpoint")
			}
		}
	}

	// The guard on the guard. Every count is literal, because a walk that
	// stopped finding packages, a population that stopped recognising the actor
	// and an exemption map that grew over the whole population all pass this
	// test by asserting nothing whatsoever.
	require.Greater(t, packages, 2, "the walk found almost no service packages; it is not looking where it thinks it is")
	require.Greater(t, population, 20, "almost no method takes an access.Actor: the population predicate has stopped recognising one")
	require.Greater(t, checked, 12, "almost the whole population is exempt: checkpointExempt has become an off switch for the gate")
	require.Greater(t, holders, 0,
		"nothing holds an authorization port any more, so no method below can be reaching one")

	sort.Strings(offences)
	assert.Empty(t, offences)
}

// The exemption map read the other way. A method excused from the checkpoint
// must still reach the refusal its reason names, and one that has since started
// reaching the checkpoint has to be struck out rather than left holding a
// licence forever (the pattern internal/web/page/served_test.go establishes).
func TestEveryExemptionIsStillTrueAndStillNeeded(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	seen := make(map[string]bool, len(checkpointExempt))

	var anchored int

	for _, dir := range servicePackages(t, root) {
		if _, outside := checkpointPackagesOutsideTheRule[dir]; outside {
			continue
		}

		facts := analyse(t, root, dir)

		for _, key := range facts.population() {
			excuse, exempt := checkpointExempt[dir+"."+key]
			if !exempt {
				continue
			}

			seen[dir+"."+key] = true

			t.Run(dir+"."+key, func(t *testing.T) {
				assert.Falsef(t, facts.reachesTheCheckpoint(key),
					"exempt (%s) and now reaches the checkpoint: strike it out of checkpointExempt", excuse.reason)

				if excuse.insteadReaches == "" {
					return
				}

				assert.Truef(t, facts.reaches(key, named(excuse.insteadReaches)),
					"exempt because it reaches %s (%s), and it does not",
					excuse.insteadReaches, excuse.reason)
			})

			if excuse.insteadReaches != "" {
				anchored++
			}
		}
	}

	for key := range checkpointExempt {
		assert.Truef(t, seen[key], "%s is exempt and no longer exists: strike it out of checkpointExempt", key)
	}

	// The same discipline on the package map, and it is not decoration: written
	// against a one-level walk, two of its four entries named packages the walk
	// never reached, so they excused nothing and nothing said so.
	reached := map[string]bool{}
	for _, dir := range servicePackages(t, repoRoot(t)) {
		reached[dir] = true
	}

	for dir := range checkpointPackagesOutsideTheRule {
		assert.Truef(t, reached[dir], "%s is skipped and the walk never reaches it: strike it out of checkpointPackagesOutsideTheRule", dir)
	}

	require.Greater(t, anchored, 6,
		"almost no exemption names a refusal it reaches instead: the map has decayed into prose")
}

// ---------------------------------------------------------------------------
// The walk
// ---------------------------------------------------------------------------

// servicePackages is every package directory under the walked trees, in
// slash-relative form, sorted. Directories are what this walk is about — the
// rule is stated per package, because a package's own declared ports are what
// decide whether a method reaches a checkpoint or hands the actor on.
//
// It descends to the bottom rather than one level. Written a level deep it
// found four packages and silently missed medicationtest and identitytest,
// whose entries in the skip map above were then licences for nothing — which is
// why every entry in that map is asserted to have been reached.
func servicePackages(t *testing.T, root string) []string {
	t.Helper()

	found := map[string]bool{}

	for _, tree := range checkpointRoots {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(tree)), func(abs string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.IsDir() {
				return nil
			}

			if strings.HasPrefix(entry.Name(), ".") && filepath.ToSlash(abs) != filepath.ToSlash(filepath.Join(root, tree)) {
				return fs.SkipDir
			}

			if len(goFiles(t, abs)) == 0 {
				return nil
			}

			rel, relErr := filepath.Rel(root, abs)
			if relErr != nil {
				return relErr
			}

			found[filepath.ToSlash(rel)] = true

			return nil
		})
		require.NoErrorf(t, err, "walking %s", tree)
	}

	dirs := make([]string, 0, len(found))
	for dir := range found {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	return dirs
}

// goFiles is one directory's authored Go, sorted. `_test.go` is excluded: the
// rule is about what the application does, and a test that calls a service
// method directly is the caller, not the method.
func goFiles(t *testing.T, abs string) []string {
	t.Helper()

	entries, err := os.ReadDir(abs)
	require.NoErrorf(t, err, "reading %s", abs)

	var files []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || path.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}

		files = append(files, filepath.Join(abs, name))
	}

	sort.Strings(files)

	return files
}

// ---------------------------------------------------------------------------
// One package's facts
// ---------------------------------------------------------------------------

// declared is one function or method with the import aliases of the file it was
// written in, so that a renamed import cannot walk past any rule below.
type declared struct {
	decl    *ast.FuncDecl
	aliases map[string]string

	// receiver is the identifier the method calls itself by, empty for a plain
	// function. Resolving `s.helper(...)` needs it.
	receiver     string
	receiverType string

	// actor is the parameter name the access.Actor arrived under, empty when
	// the method takes none or takes it as `_`.
	actor string
}

type packageFacts struct {
	dir string

	// decls is keyed "Type.Method" for a method and "Func" for a plain
	// function. Both are followed, because a package-local helper is how the
	// checkpoint is actually reached.
	decls map[string]declared

	// fieldTypes maps a struct type to its fields' package-local type names,
	// with pointers and slices stripped. A field whose type is not declared in
	// this package is absent.
	fieldTypes map[string]map[string]string

	// checkpointFields maps a struct type to the fields that hold an
	// authorization port.
	checkpointFields map[string]map[string]bool

	// checkpointMethods is what an authorization port answers to.
	checkpointMethods map[string]bool

	// delegationMethods is the method names of every OTHER seam this package
	// declares that itself takes an access.Actor — the ones a method may
	// discharge the rule by handing the actor to.
	delegationMethods map[string]bool

	// serviceFields is which struct fields hold a service from another
	// internal/service package: an adapter that hands the actor to one has
	// discharged the rule there.
	serviceFields map[string]map[string]bool
}

func analyse(t *testing.T, root, dir string) *packageFacts {
	t.Helper()

	facts := &packageFacts{
		dir:               dir,
		decls:             map[string]declared{},
		fieldTypes:        map[string]map[string]string{},
		checkpointFields:  map[string]map[string]bool{},
		checkpointMethods: map[string]bool{},
		delegationMethods: map[string]bool{},
		serviceFields:     map[string]map[string]bool{},
	}

	fileSet := token.NewFileSet()
	parsed := map[string]*ast.File{}
	aliases := map[string]map[string]string{}

	for _, file := range goFiles(t, filepath.Join(root, filepath.FromSlash(dir))) {
		syntax, err := parser.ParseFile(fileSet, file, nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "parsing %s", file)

		parsed[file] = syntax
		aliases[file] = importAliases(syntax)
	}

	// Interfaces and struct fields first: which method names name a checkpoint
	// and which name a delegation seam is a whole-package fact, and it decides
	// how every body below is read.
	checkpointPorts := map[string]bool{}
	actorPorts := map[string][]string{}

	for file, syntax := range parsed {
		eachInterface(syntax, func(name string, iface *ast.InterfaceType) {
			var takesActor []string

			for _, method := range iface.Methods.List {
				signature, isFunc := method.Type.(*ast.FuncType)
				if !isFunc || len(method.Names) == 0 {
					continue
				}

				if returnsGrant(signature, aliases[file]) {
					checkpointPorts[name] = true
				}

				if takesActorParam(signature, aliases[file]) != nil {
					takesActor = append(takesActor, method.Names[0].Name)
				}
			}

			actorPorts[name] = takesActor
		})

		eachStruct(syntax, func(name string, structure *ast.StructType) {
			for _, field := range structure.Fields.List {
				local, pkg, typeName := resolveType(field.Type, aliases[file])

				for _, ident := range field.Names {
					if local != "" {
						if facts.fieldTypes[name] == nil {
							facts.fieldTypes[name] = map[string]string{}
						}

						facts.fieldTypes[name][ident.Name] = local
					}

					if pkg == checkpointPackage && typeName == checkpointType {
						mark(facts.checkpointFields, name, ident.Name)
						facts.checkpointMethods["Record"] = true
						facts.checkpointMethods["Kind"] = true
					} else if strings.HasPrefix(pkg, "medikube/internal/service/") {
						mark(facts.serviceFields, name, ident.Name)
					}
				}
			}
		})
	}

	for port := range checkpointPorts {
		for _, method := range actorPorts[port] {
			facts.checkpointMethods[method] = true
		}
	}

	for port, methods := range actorPorts {
		if checkpointPorts[port] {
			continue
		}

		for _, method := range methods {
			facts.delegationMethods[method] = true
		}
	}

	// Now the fields that hold one of this package's own checkpoint ports.
	for file, syntax := range parsed {
		eachStruct(syntax, func(name string, structure *ast.StructType) {
			for _, field := range structure.Fields.List {
				local, _, _ := resolveType(field.Type, aliases[file])
				if !checkpointPorts[local] {
					continue
				}

				for _, ident := range field.Names {
					mark(facts.checkpointFields, name, ident.Name)
				}
			}
		})
	}

	for file, syntax := range parsed {
		for _, decl := range syntax.Decls {
			function, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || function.Body == nil {
				continue
			}

			entry := declared{decl: function, aliases: aliases[file]}
			key := function.Name.Name

			if function.Recv != nil && len(function.Recv.List) == 1 {
				receiver := function.Recv.List[0]

				_, _, entry.receiverType = resolveType(receiver.Type, aliases[file])
				if len(receiver.Names) == 1 {
					entry.receiver = receiver.Names[0].Name
				}

				key = entry.receiverType + "." + function.Name.Name
			}

			if name := takesActorParam(function.Type, aliases[file]); name != nil {
				entry.actor = *name
			}

			facts.decls[key] = entry
		}
	}

	return facts
}

// population is every exported method that takes an access.Actor, sorted.
//
// Taking one is what makes a method part of this rule: it is how a method is
// told whose data it is about, and a method that reaches a person's record
// without one could not know whose it was.
func (p *packageFacts) population() []string {
	var keys []string

	for key, entry := range p.decls {
		if entry.decl.Recv == nil || !entry.decl.Name.IsExported() {
			continue
		}

		if takesActorParam(entry.decl.Type, entry.aliases) == nil {
			continue
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func (p *packageFacts) reachesTheCheckpoint(key string) bool {
	return p.reaches(key, func(call *ast.CallExpr, in declared) bool {
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return false
		}

		if p.checkpointCall(selector, in) {
			return true
		}

		if p.serviceCall(selector, in) && passes(call, in.actor) {
			return true
		}

		return p.delegationMethods[selector.Sel.Name] && passes(call, in.actor)
	})
}

// checkpointCall is `<receiver>.<field>.<Method>` where the field holds an
// authorization port and the method is one of its own.
//
// The field is what does the work, and it has to: this package's audit port
// declares a method called Record too, and `s.auditor.Record(ctx, event)` is a
// call to the audit trail. A rule that matched on the method name alone would
// read every refusal it wrote as the authorization it was recording.
func (p *packageFacts) checkpointCall(selector *ast.SelectorExpr, in declared) bool {
	if !p.checkpointMethods[selector.Sel.Name] {
		return false
	}

	field, isField := selector.X.(*ast.SelectorExpr)
	if !isField {
		return false
	}

	holder, isIdent := field.X.(*ast.Ident)
	if !isIdent || holder.Name != in.receiver || in.receiver == "" {
		return false
	}

	return p.checkpointFields[in.receiverType][field.Sel.Name]
}

// serviceCall is `<receiver>.<field>.<Method>` where the field holds another
// service package's type.
func (p *packageFacts) serviceCall(selector *ast.SelectorExpr, in declared) bool {
	field, isField := selector.X.(*ast.SelectorExpr)
	if !isField {
		return false
	}

	holder, isIdent := field.X.(*ast.Ident)
	if !isIdent || holder.Name != in.receiver || in.receiver == "" {
		return false
	}

	return p.serviceFields[in.receiverType][field.Sel.Name]
}

// named matches a call by the name being called, for the counter-assertions the
// exemptions carry. It is deliberately looser than checkpointCall: it is used
// to prove an exemption's claim rather than to grant one, so a false match
// weakens only the exemption's own evidence and never the gate.
func named(target string) func(*ast.CallExpr, declared) bool {
	return func(call *ast.CallExpr, _ declared) bool {
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			return callee.Name == target
		case *ast.SelectorExpr:
			return callee.Sel.Name == target
		default:
			return false
		}
	}
}

// reaches walks the call graph rooted at one declaration, following only calls
// this package can resolve, and reports whether hit answered for any of them.
func (p *packageFacts) reaches(key string, hit func(*ast.CallExpr, declared) bool) bool {
	return p.walk(key, hit, map[string]bool{})
}

func (p *packageFacts) walk(key string, hit func(*ast.CallExpr, declared) bool, seen map[string]bool) bool {
	if seen[key] {
		return false
	}

	seen[key] = true

	entry, known := p.decls[key]
	if !known {
		return false
	}

	found := false

	ast.Inspect(entry.decl.Body, func(node ast.Node) bool {
		if found {
			return false
		}

		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		if hit(call, entry) {
			found = true

			return false
		}

		if next, resolvable := p.callee(call, entry); resolvable && p.walk(next, hit, seen) {
			found = true

			return false
		}

		return true
	})

	return found
}

// callee resolves a call to a declaration in this package, and only the two
// shapes it can resolve without a type checker: a plain function, and a method
// reached through the enclosing receiver — `s.helper(...)`, or `a.service.M(...)`
// where the field's type is declared here too.
//
// Anything else resolves to nothing, which is the safe direction: an
// unresolvable call cannot be how a method is judged to have authorized.
func (p *packageFacts) callee(call *ast.CallExpr, in declared) (string, bool) {
	switch callee := call.Fun.(type) {
	case *ast.Ident:
		if _, known := p.decls[callee.Name]; known {
			return callee.Name, true
		}
	case *ast.SelectorExpr:
		switch receiver := callee.X.(type) {
		case *ast.Ident:
			if in.receiver != "" && receiver.Name == in.receiver {
				return in.receiverType + "." + callee.Sel.Name, true
			}
		case *ast.SelectorExpr:
			holder, isIdent := receiver.X.(*ast.Ident)
			if !isIdent || in.receiver == "" || holder.Name != in.receiver {
				return "", false
			}

			if local := p.fieldTypes[in.receiverType][receiver.Sel.Name]; local != "" {
				return local + "." + callee.Sel.Name, true
			}
		}
	}

	return "", false
}

// ---------------------------------------------------------------------------
// Syntax
// ---------------------------------------------------------------------------

func passes(call *ast.CallExpr, actor string) bool {
	if actor == "" {
		return false
	}

	for _, argument := range call.Args {
		if ident, isIdent := argument.(*ast.Ident); isIdent && ident.Name == actor {
			return true
		}
	}

	return false
}

// takesActorParam answers the parameter name the actor arrived under, nil when
// there is none. A named result is a pointer rather than a bool-and-string
// because `_` is a parameter that exists and cannot be passed on.
func takesActorParam(signature *ast.FuncType, aliases map[string]string) *string {
	if signature.Params == nil {
		return nil
	}

	for _, param := range signature.Params.List {
		_, pkg, name := resolveType(param.Type, aliases)
		if pkg != actorPackage || name != actorType {
			continue
		}

		found := ""

		if len(param.Names) > 0 && param.Names[0].Name != "_" {
			found = param.Names[0].Name
		}

		return &found
	}

	return nil
}

func returnsGrant(signature *ast.FuncType, aliases map[string]string) bool {
	if signature.Results == nil {
		return false
	}

	for _, result := range signature.Results.List {
		if _, pkg, name := resolveType(result.Type, aliases); pkg == actorPackage && name == grantType {
			return true
		}
	}

	return false
}

// resolveType strips pointers and slices and answers three things: the
// package-local type name if the type is declared in this package, and the
// import path and name if it is qualified.
func resolveType(expr ast.Expr, aliases map[string]string) (local, pkg, name string) {
	for {
		switch typed := expr.(type) {
		case *ast.StarExpr:
			expr = typed.X
		case *ast.ArrayType:
			expr = typed.Elt
		case *ast.Ident:
			return typed.Name, "", typed.Name
		case *ast.SelectorExpr:
			qualifier, isIdent := typed.X.(*ast.Ident)
			if !isIdent {
				return "", "", ""
			}

			return "", aliases[qualifier.Name], typed.Sel.Name
		default:
			return "", "", ""
		}
	}
}

func eachInterface(file *ast.File, visit func(name string, iface *ast.InterfaceType)) {
	eachType(file, func(name string, expr ast.Expr) {
		if iface, isInterface := expr.(*ast.InterfaceType); isInterface {
			visit(name, iface)
		}
	})
}

func eachStruct(file *ast.File, visit func(name string, structure *ast.StructType)) {
	eachType(file, func(name string, expr ast.Expr) {
		if structure, isStruct := expr.(*ast.StructType); isStruct {
			visit(name, structure)
		}
	})
}

func eachType(file *ast.File, visit func(name string, expr ast.Expr)) {
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.TYPE {
			continue
		}

		for _, spec := range general.Specs {
			if typed, isType := spec.(*ast.TypeSpec); isType {
				visit(typed.Name.Name, typed.Type)
			}
		}
	}
}

// importAliases maps the identifier a file actually uses to the package it
// names, so that a renamed import cannot walk past any of the rules above. The
// same shape as internal/logging/singlestream_test.go:280.
func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string, len(file.Imports))

	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		name := imported
		if slash := strings.LastIndex(name, "/"); slash >= 0 {
			name = name[slash+1:]
		}

		if spec.Name != nil {
			name = spec.Name.Name
		}

		if name == "_" || name == "." {
			continue
		}

		aliases[name] = imported
	}

	return aliases
}

func mark(table map[string]map[string]bool, outer, inner string) {
	if table[outer] == nil {
		table[outer] = map[string]bool{}
	}

	table[outer][inner] = true
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
