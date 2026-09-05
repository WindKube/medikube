package openapi

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

// The per-operation half of the document.
//
// internal/httproute.Route carries what the ROUTER needs — method, path, kind,
// auth, summary — and deliberately nothing a client would need: no parameters,
// no bodies, no status codes. That detail lives here, keyed by the same OpID,
// and Generate refuses in both directions: a route with no entry cannot be
// documented, and an entry no route serves cannot be published. So this table
// is hand-written and it cannot rot silently, which is the only kind of
// hand-written inventory Principle VIII allows.
//
// Every status, parameter and header below is transcribed from
// specs/001-walking-skeleton/contracts/. Changing one is a contract change.

// param is one parameter, in the query or in a header.
type param struct {
	name        string
	description string
	schema      *openapi3.Schema
	required    bool
}

// operationDoc is everything about an operation that is not in its route.
type operationDoc struct {
	// The documented success. successBody names a component; empty documents
	// the status and its meaning without a body, which is what an operation
	// whose DTO has not landed yet honestly says.
	successStatus  int
	successNote    string
	successBody    string
	successType    string
	successHeaders []string

	// The documented failures. Each carries the shared error envelope.
	errors []int

	// The documented failures that deliberately do NOT carry the envelope,
	// each with the contract that makes it an exception. Anything not listed
	// here and not in errors cannot appear at all.
	nonEnvelope map[int]string

	query       []param
	headers     []param
	requestBody string

	// ownerScoped operations resolve a record or a kind out of the stored data,
	// so their published authorization rule has to state the 404 that a refusal
	// and a genuine absence share (FR-033).
	ownerScoped bool

	// notes is appended to the authorization rule, which is what opens every
	// description.
	notes string
}

// The response headers this phase documents, described once.
var responseHeaderDescriptions = map[string]string{
	"Location": "The address of the created resource.",
	"ETag":     "The record's current version, derived from `updated`. It is what `If-Match` is compared against.",
}

// contracts/README.md's status table, one description per status. The document
// says which machine code a status carries so a client can branch on the code
// rather than on the prose.
var errorDescriptions = map[int]string{
	http.StatusBadRequest: "`invalid_token` for a recovery or confirmation token that is expired, already used or " +
		"tampered with — one code for all three, so no token's former existence is disclosed — or `invalid_cursor` " +
		"for a forged, tampered or unparseable cursor.",
	http.StatusUnauthorized:       "`unauthenticated`. The body names no account and no resource.",
	http.StatusForbidden:          "`registration_closed`, or `forbidden` where the resource's existence is already known to the caller. Never returned for a clinical record.",
	http.StatusNotFound:           "`not_found`. On owner-scoped data this is also the answer for a record belonging to somebody else, byte-identical apart from `request_id` to an id that never existed (FR-033).",
	http.StatusConflict:           "`conflict`. The message names no address and no record.",
	http.StatusPreconditionFailed: "`version_mismatch`: the `If-Match` precondition did not match the stored version.",
	http.StatusUnprocessableEntity: "`validation_failed`, with every offending member reported at once in `fields[]`. " +
		"An unknown or unexpected member is reported with the field code `unknown_field`.",
	http.StatusRequestEntityTooLarge: "`payload_too_large`: the upload exceeds the configured maximum.",
	http.StatusUnsupportedMediaType:  "`unsupported_media_type`: the sniffed content type is not one this instance accepts.",
	http.StatusTooManyRequests:       "`rate_limited`.",
	http.StatusInternalServerError:   "`internal_error`. The message is a constant: no internal error string ever reaches a client.",
	http.StatusServiceUnavailable:    "`mail_unconfigured`: this instance has no outgoing mail configured, so the request is refused rather than accepted as though it had succeeded (FR-076).",
}

func stringParam(name, description string) param {
	return param{name: name, description: description, schema: primitive("string")}
}

func requiredParam(p param) param {
	p.required = true

	return p
}

func listParam(name, description string) param {
	return param{
		name:        name,
		description: description + " Comma-separated; whitespace is not trimmed.",
		schema:      primitive("string"),
	}
}

func dateParam(name, description string) param {
	schema := primitive("string")
	schema.Format = "date"

	return param{name: name, description: description, schema: schema}
}

func boolParam(name, description string) param {
	return param{name: name, description: description, schema: primitive("boolean")}
}

func limitParam() param {
	schema := primitive("integer")
	schema.Min = openapi3.Ptr(1.0)
	schema.Max = openapi3.Ptr(100.0)
	schema.Default = 25

	return param{name: "limit", description: "Page size, 1..100.", schema: schema}
}

func cursorParam() param {
	return stringParam("cursor",
		"An opaque, server-minted continuation token encoding the sort keys and the last id — never an offset, "+
			"so a row inserted or deleted between two pages cannot shift a boundary. A forged or unparseable cursor is 400.")
}

func countParam() param {
	return boolParam("count", "Include `total` in the envelope. Absent means the count is not computed.")
}

// pagedListDoc is the shape contracts/records.md gives both list operations.
func pagedListDoc(query []param, notes string) operationDoc {
	return operationDoc{
		successStatus: http.StatusOK,
		successNote:   "A page of the signed-in account's records.",
		successBody:   RecordSummaryPageSchema,
		errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusUnprocessableEntity,
			http.StatusInternalServerError,
		},
		query:       append(query, limitParam(), cursorParam(), countParam()),
		ownerScoped: true,
		notes:       notes,
	}
}

// nativeDoc describes one of the PocketBase-native paths contracts/README.md
// deliberately leaves reachable. PocketBase serves them and answers failures in
// its own error shape, which MediKube neither defines nor may claim to, so no
// failure is documented here — only that the path exists and why.
func nativeDoc(note string) operationDoc {
	return operationDoc{
		successStatus: http.StatusOK,
		successNote:   "PocketBase's own response.",
		notes: "Served by PocketBase, not by MediKube. It is documented so that a reachable path is never one " +
			"nobody wrote down, and its failures carry PocketBase's error shape rather than MediKube's. " + note,
	}
}

//nolint:funlen // it is a table: one entry per operation, transcribed from contracts/.
func operationDocs() map[string]operationDoc {
	ifMatch := requiredParam(stringParam("If-Match",
		"The version the caller last saw, from the record's `ETag`. Required: an optional precondition is a "+
			"precondition nobody sends. A mismatch is 412."))

	docs := map[string]operationDoc{
		// contracts/auth.md
		"getAuthConfig": {
			successStatus: http.StatusOK,
			successNote:   "Whether registration is open and the password rules to publish.",
			errors:        []int{http.StatusInternalServerError},
			notes:         "It reveals nothing about any account: not how many exist, not whether a given address is registered.",
		},
		"register": {
			successStatus:  http.StatusCreated,
			successNote:    "The account was created and signed in, in one transaction.",
			successHeaders: []string{"Location"},
			errors: []int{
				http.StatusForbidden,
				http.StatusConflict,
				http.StatusUnprocessableEntity,
				http.StatusTooManyRequests,
				http.StatusInternalServerError,
			},
			notes: "The conflict message names no address: confirming that a specific person has an account here is itself a disclosure.",
		},
		"login": {
			successStatus: http.StatusOK,
			successNote:   "A session.",
			errors:        []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError},
			notes: "An unknown address, a wrong password and a disabled account produce the identical 401, byte-identical " +
				"apart from `request_id`, so none of the three can be probed (FR-005).",
		},
		"refreshSession": {
			successStatus: http.StatusOK,
			successNote:   "A fresh session.",
			errors:        []int{http.StatusUnauthorized, http.StatusInternalServerError},
			notes:         "A token invalidated by a password change or a sign-out elsewhere is refused here like any other.",
		},
		"logout": {
			successStatus: http.StatusNoContent,
			successNote:   "The session ended.",
			errors:        []int{http.StatusUnauthorized, http.StatusInternalServerError},
			notes:         "Every session the account still had open ends too, which is what FR-007 asks for.",
		},
		"requestPasswordReset": {
			successStatus: http.StatusAccepted,
			successNote:   "Accepted. The body is the same whether or not the address has an account.",
			errors:        []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusInternalServerError},
			notes: "A recovery form that answers \"no such account\" is an account-existence oracle, so all three " +
				"outcomes answer identically apart from `request_id` (FR-073).",
		},
		"confirmPasswordReset": {
			successStatus: http.StatusNoContent,
			successNote:   "The password was set and every session issued before the change stopped working.",
			errors: []int{
				http.StatusBadRequest,
				http.StatusUnprocessableEntity,
				http.StatusTooManyRequests,
				http.StatusInternalServerError,
			},
			notes: "Expired, already used and tampered-with tokens share one refusal; distinguishing them would tell a caller which tokens once existed.",
		},
		"requestEmailVerification": {
			successStatus: http.StatusAccepted,
			successNote:   "Accepted. An address that is already confirmed answers the same way.",
			errors: []int{
				http.StatusUnauthorized,
				http.StatusTooManyRequests,
				http.StatusServiceUnavailable,
				http.StatusInternalServerError,
			},
			notes: "It takes no body: the address is read from the authenticated record, because accepting one would let a caller aim this instance's mailer at a stranger.",
		},
		"confirmEmailVerification": {
			successStatus: http.StatusNoContent,
			successNote:   "The address is confirmed.",
			errors:        []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusInternalServerError},
			notes:         "Public, because the person following the link may not be signed in on that device.",
		},

		// contracts/account.md
		"getMe": {
			successStatus: http.StatusOK,
			successNote:   "The signed-in account.",
			errors:        []int{http.StatusUnauthorized, http.StatusInternalServerError},
			notes:         "There is no id to supply and no other account this can reach.",
		},
		"updateMe": {
			successStatus: http.StatusOK,
			successNote:   "The updated account. The response is also the feedback the settings page renders.",
			errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
			notes:         "The address, the role and the disabled state are not members of the request body, so they cannot be changed here at all (FR-012).",
		},
		"deleteMe": {
			successStatus: http.StatusNoContent,
			successNote:   "The account and everything it owns are permanently gone.",
			errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
			notes:         "It requires the current password and a typed confirmation phrase, and it cannot be undone.",
		},
		"changePassword": {
			successStatus: http.StatusNoContent,
			successNote:   "The password changed and every other session stopped working.",
			errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
			notes:         "A wrong current password is 422 on that member rather than 401: the caller is authenticated, the password is what failed.",
		},

		// contracts/records.md
		"listRecords": pagedListDoc(
			[]param{
				listParam("kind", "Restrict to these registered path segments. Absent means every kind."),
				dateParam("from", "Narrow to records whose primary date is on or after this day."),
				dateParam("to", "Narrow to records whose primary date is on or before this day."),
			},
			"The cross-kind list. Its items are the record union, so a client written against it today keeps working when more kinds are registered.",
		),
		"listRecordsOfKind": pagedListDoc(
			[]param{
				stringParam("q", "Case-insensitive substring match over the kind's searchable text."),
				listParam("status", "Restrict to these status values."),
				stringParam("sort", "Drawn from the kind's allowlist, with a `-` prefix for descending. "+
					"A value outside the allowlist is 422 and is never silently ignored."),
			},
			"A {"+KindPathParameter+"} outside the enum is 404 and not 400: an unregistered kind is indistinguishable from a path that does not exist.",
		),
		"createRecord": {
			successStatus:  http.StatusCreated,
			successNote:    "The created record.",
			successBody:    RecordSchema,
			successHeaders: []string{"Location", "ETag"},
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			requestBody: RecordCreateSchema,
			ownerScoped: true,
			notes: "The owner is taken from the session. The request body has no owner member at all, so re-attribution " +
				"is impossible by shape rather than by a runtime check (FR-032).",
		},
		"getRecord": {
			successStatus:  http.StatusOK,
			successNote:    "The record.",
			successBody:    RecordSchema,
			successHeaders: []string{"ETag"},
			errors:         []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
			ownerScoped:    true,
			notes:          "Members that were never filled in are absent from the response rather than present and empty (FR-024).",
		},
		"updateRecord": {
			successStatus:  http.StatusOK,
			successNote:    "The updated record.",
			successBody:    RecordSchema,
			successHeaders: []string{"ETag"},
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusPreconditionFailed,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			headers:     []param{ifMatch},
			requestBody: RecordPatchSchema,
			ownerScoped: true,
			notes: "Only supplied members change. A missing `If-Match` is 422 on that header; a mismatch is 412 " +
				"and the response carries the stored record's current version.",
		},
		"deleteRecord": {
			successStatus: http.StatusNoContent,
			successNote:   "The record is permanently gone. There is no recycle bin and no undo.",
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusPreconditionFailed,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			headers:     []param{ifMatch},
			ownerScoped: true,
			notes:       "`If-Match` is required for the same reason as on update: deleting the record you last saw is a different act from deleting whatever is there now.",
		},

		// contracts/patients.md
		"listPatients": {
			successStatus: http.StatusOK,
			successNote:   "A page of the signed-in account's patients, `total` and `owned_count` always present.",
			errors: []int{
				http.StatusBadRequest,
				http.StatusUnauthorized,
				http.StatusInternalServerError,
			},
			query:       []param{stringParam("q", "Case-insensitive substring over first and last name."), limitParam(), cursorParam()},
			ownerScoped: true,
			notes:       "Sorted `last_name, first_name, id` ascending; not configurable in this phase. Never empty in practice: the self-record guarantees one row (FR-005).",
		},
		"createPatient": {
			successStatus:  http.StatusCreated,
			successNote:    "The created patient.",
			successHeaders: []string{"Location", "ETag"},
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			ownerScoped: true,
			notes: "The owner is taken from the session and `is_self_record` is always false; neither is a member of " +
				"the request body, so re-attribution and a second self-record are impossible by shape (FR-002, FR-004). " +
				"A `primary_practitioner` the actor does not own is 404.",
		},
		"getPatient": {
			successStatus:  http.StatusOK,
			successNote:    "The patient.",
			successHeaders: []string{"ETag"},
			errors:         []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
			ownerScoped:    true,
			notes:          "Members that were never filled in are absent or null, never a value that reads as recorded (FR-030).",
		},
		"updatePatient": {
			successStatus:  http.StatusOK,
			successNote:    "The updated patient.",
			successHeaders: []string{"ETag"},
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusPreconditionFailed,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			headers:     []param{ifMatch},
			ownerScoped: true,
			notes: "Only supplied members change. A missing `If-Match` is 422 on that header; a mismatch is 412 and " +
				"the response carries the stored patient's current representation.",
		},

		// contracts/patient-photo.md
		"putPatientPhoto": {
			successStatus: http.StatusOK,
			successNote:   "The photo was stored and both thumbnails were generated eagerly, before any request for them.",
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusUnprocessableEntity,
				http.StatusRequestEntityTooLarge,
				http.StatusUnsupportedMediaType,
				http.StatusInternalServerError,
			},
			ownerScoped: true,
			notes: "`multipart/form-data`, one part named `photo`. The type is sniffed from the bytes, never trusted " +
				"from the declared Content-Type or the filename (FR-008). Replacing removes the previous file and its " +
				"thumbnails; the previous photograph is not retrievable afterwards (FR-008, US1-5). The uploaded " +
				"filename is PHI and never appears in a response or a log line (FR-046).",
		},
		"getPatientPhoto": {
			successStatus: http.StatusOK,
			successNote:   "The image bytes.",
			errors: []int{
				http.StatusUnauthorized,
				http.StatusNotFound,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			query:       []param{stringParam("size", "`original`, `100x100t` or `400x400f`, default `100x100t`. Any other value is 422.")},
			ownerScoped: true,
			notes: "Not owned, does not exist, and no photo recorded are the identical 404 (FR-042, FR-044). Served " +
				"only through this route: never PocketBase's own file-token mechanism (FR-044).",
		},
		"deletePatientPhoto": {
			successStatus: http.StatusNoContent,
			successNote:   "The photo and both thumbnails are gone. Removing a patient with no photo is also 204.",
			errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
			ownerScoped:   true,
			notes:         "Hard deleted; there is no recycle bin.",
		},

		// contracts/streams.md
		"streamRecords": {
			successStatus: http.StatusOK,
			successNote: "A Datastar element stream. Two event names are sent and no others: " +
				"`datastar-patch-elements` for a record that was created, changed or removed, and " +
				"`datastar-patch-signals` for the heartbeat.",
			successType: "text/event-stream",
			// 422 is here because the `kind` parameter is refused rather than
			// ignored: a stream that silently narrowed to nothing is a live
			// view that never updates and never says why. It is the same
			// asymmetry the cross-kind list draws — an unregistered value in a
			// query parameter names itself, because a caller who reached the
			// parameter already knows the path exists.
			errors: []int{
				http.StatusUnauthorized,
				http.StatusUnprocessableEntity,
				http.StatusInternalServerError,
			},
			query: []param{
				listParam("kind", "Restrict to these registered path segments. Absent means every kind."),
			},
			ownerScoped: true,
			notes: "Anonymous is refused before the stream opens: a 401 delivered as an event would be " +
				"indistinguishable from a working stream that never sends anything. Every event is re-authorised " +
				"for the subscriber, and the stream carries ids rather than record bodies.",
		},

		// contracts/health.md
		"healthz": {
			successStatus: http.StatusOK,
			successNote:   "The process is running.",
			errors:        []int{http.StatusInternalServerError},
			notes:         "It touches no database and no filesystem, and its only other outcome is not answering at all (FR-052).",
		},
		"readyz": {
			successStatus: http.StatusOK,
			successNote:   "The instance can serve. The check vocabulary is fixed and closed: `database`, `migrations`, `storage`, each `ok` or `error`.",
			errors:        []int{http.StatusInternalServerError},
			nonEnvelope: map[int]string{
				http.StatusServiceUnavailable: "Not ready, or draining. The body is the readiness payload rather than an " +
					"error envelope, and it has no message member at all: a driver error can carry a file path, a DSN or " +
					"a credential, so the reason is a check name and the error itself goes to the log (FR-052).",
			},
			notes: "It reports whether the instance can serve and nothing about the data it holds.",
		},

		// contracts/README.md, "Documented PocketBase-native paths that stay public"
		"nativeAdminUI":                   nativeDoc("It ships in production, hardened: mandatory superuser MFA, mandatory IP allowlist, every session audited."),
		"nativeSuperuserAuthWithPassword": nativeDoc("The admin UI's own authentication."),
		"nativeSuperuserAuthRefresh":      nativeDoc("The admin UI's own session refresh."),
		"nativeSuperuserAuthMethods":      nativeDoc("The admin UI's own auth-method discovery."),
		"nativeUserAuthWithPassword": nativeDoc("The lockdown is scoped to the record subtree precisely so this survives. " +
			"MediKube's own sign-in is the supported path, and both are audited from the same hook."),
		"nativeUserAuthRefresh": nativeDoc("PocketBase-native session refresh."),
		"nativeUserAuthMethods": nativeDoc("PocketBase-native auth-method discovery."),
		"nativeUserRequestPasswordReset": nativeDoc("It enforces the same token rules as MediKube's own recovery path, " +
			"because it is the same code that path calls."),
		"nativeUserConfirmPasswordReset": nativeDoc("PocketBase-native recovery confirmation."),
		"nativeUserRequestVerification":  nativeDoc("PocketBase-native confirmation request."),
		"nativeUserConfirmVerification":  nativeDoc("PocketBase-native confirmation."),
	}

	return docs
}
