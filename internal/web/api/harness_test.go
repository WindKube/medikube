package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// The addresses under test, composed from the kind table and never spelled: the
// plural is declared once, in internal/domain/kind, and an assertion that wrote
// it by hand would keep passing against a path the router had stopped serving.
func collectionURL() string { return "/api/v1/records/" + kind.Medication.Segment() }

func recordURL(id string) string { return collectionURL() + "/" + id }

func crossKindURL() string { return "/api/v1/records" }

// missingID is an identifier of the right shape that no fixture uses. It is
// what a genuine not-found is produced with, and FR-033's comparison is against
// the response it produces.
const missingID = "mkmednobody0001"

// caller drives requests through the instance a single test owns.
//
// One instance per caller, never one shared between two: apis.NewRouter binds
// an anonymous OnServe handler per construction and hook.Bind appends when no
// Id is given, so a reused app deepens the middleware chain — which is real
// stack frames, not a loop — until the goroutine stack ends the process
// (reconciliation C14).
type caller struct {
	t       *testing.T
	app     *tests.TestApp
	handler http.Handler
	token   string
}

func newCaller(t *testing.T) *caller {
	t.Helper()

	return newCallerAs(t, testsupport.AccountAEmail)
}

func newCallerAs(t *testing.T, email string) *caller {
	t.Helper()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	return &caller{
		t:       t,
		app:     instance.App,
		handler: handler,
		token:   testsupport.UserToken(t, instance.App, email),
	}
}

// as returns a second caller onto the SAME instance, signed in as somebody
// else. The two share stored data, which is the whole point of an isolation
// assertion: two accounts on one instance is the arrangement a leak happens in.
func (c *caller) as(email string) *caller {
	c.t.Helper()

	return &caller{
		t:       c.t,
		app:     c.app,
		handler: c.handler,
		token:   testsupport.UserToken(c.t, c.app, email),
	}
}

// anonymous is the same instance with no credentials.
func (c *caller) anonymous() *caller {
	return &caller{t: c.t, app: c.app, handler: c.handler}
}

// response is one answer, read whole so that every assertion can be made
// against the same bytes — which is what FR-033's byte-identical comparison
// needs and what a streamed body cannot give.
type response struct {
	Status  int
	Header  http.Header
	Body    string
	rawBody []byte
}

func (c *caller) do(method, url, body string, headers map[string]string) response {
	c.t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request := httptest.NewRequestWithContext(c.t.Context(), method, url, reader)

	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		request.Header.Set("Authorization", c.token)
	}

	for name, value := range headers {
		request.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	c.handler.ServeHTTP(recorder, request)

	result := recorder.Result()
	defer result.Body.Close()

	raw, err := io.ReadAll(result.Body)
	require.NoError(c.t, err)

	return response{Status: result.StatusCode, Header: result.Header, Body: string(raw), rawBody: raw}
}

func (c *caller) get(url string) response { return c.do(http.MethodGet, url, "", nil) }

func (c *caller) post(url, body string) response { return c.do(http.MethodPost, url, body, nil) }

func (c *caller) patch(url, body, version string) response {
	return c.do(http.MethodPatch, url, body, ifMatch(version))
}

func (c *caller) delete(url, version string) response {
	return c.do(http.MethodDelete, url, "", ifMatch(version))
}

func ifMatch(version string) map[string]string {
	if version == "" {
		return nil
	}

	return map[string]string{"If-Match": version}
}

// decode reads a JSON response into target and fails with the body in the
// message, because "cannot unmarshal" without the document says nothing.
func (r response) decode(t *testing.T, target any) {
	t.Helper()

	require.NoErrorf(t, json.Unmarshal(r.rawBody, target), "decoding %s", r.Body)
}

// medicationDTO mirrors api.Medication as a client sees it. It is declared here
// rather than imported so that a change to the published shape has to be made
// twice — once in the DTO and once in the assertion — which is what makes the
// wire format a contract instead of whatever the struct currently is.
type medicationDTO struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	Name            string  `json:"name"`
	AlternativeName string  `json:"alternative_name"`
	Type            string  `json:"type"`
	Dosage          string  `json:"dosage"`
	Frequency       string  `json:"frequency"`
	Route           string  `json:"route"`
	Indication      string  `json:"indication"`
	StartedOn       *string `json:"started_on"`
	EndedOn         *string `json:"ended_on"`
	Status          string  `json:"status"`
	SideEffects     string  `json:"side_effects"`
	Notes           string  `json:"notes"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// listDTO is contracts/README.md's one list envelope.
type listDTO struct {
	Items      []medicationDTO `json:"items"`
	NextCursor *string         `json:"next_cursor"`
	Total      *int            `json:"total"`
}

// envelopeDTO is the error envelope of contracts/README.md, and the `current`
// member the 412 adds beside it.
type envelopeDTO struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Fields    []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`

	Current medicationDTO `json:"current"`
}

func (r response) envelope(t *testing.T) envelopeDTO {
	t.Helper()

	var envelope envelopeDTO
	r.decode(t, &envelope)

	return envelope
}

func (r response) medication(t *testing.T) medicationDTO {
	t.Helper()

	var medication medicationDTO
	r.decode(t, &medication)

	return medication
}

func (r response) list(t *testing.T) listDTO {
	t.Helper()

	var page listDTO
	r.decode(t, &page)

	return page
}

// etag is the version to send back as If-Match, quoted exactly as the server
// issued it.
func (r response) etag(t *testing.T) string {
	t.Helper()

	tag := r.Header.Get("ETag")
	require.NotEmptyf(t, tag, "the response carries no ETag, so nothing can be written back against it: %s", r.Body)

	return tag
}

// stored reads a medication straight out of the database, so that an assertion
// about what was written is not made against the response that claimed to write
// it.
func (c *caller) stored(id string) (*core.Record, error) {
	return c.app.FindRecordById(kind.Medication.Collection(), id)
}

// fieldCodes is the refusal as a client would branch on it: the field and its
// machine code, in the order the envelope lists them.
func (e envelopeDTO) fieldCodes() [][2]string {
	codes := make([][2]string, 0, len(e.Error.Fields))
	for _, field := range e.Error.Fields {
		codes = append(codes, [2]string{field.Field, field.Code})
	}

	return codes
}

// activeStatus is data-model §2's default state, read from the domain so a
// renamed value fails to compile rather than failing an assertion.
const activeStatus = clinical.TherapyStatusActive

func itoa(value int) string { return strconv.Itoa(value) }

// storedInstants reads one date column for every medication in the instance, at
// the precision the database keeps it. It is what an ordering assertion over
// `updated` has to compare against: the wire renders that column as RFC3339
// with no fractional part, and the ORDER BY does not.
func (c *caller) storedInstants(column string) map[string]string {
	c.t.Helper()

	rows, err := c.app.FindAllRecords(kind.Medication.Collection())
	require.NoError(c.t, err)

	instants := make(map[string]string, len(rows))
	for _, row := range rows {
		instants[row.Id] = row.GetDateTime(column).String()
	}

	return instants
}

// storedVersion is the ETag source read from the database, so an assertion
// about the header is not made against the value the handler put in it.
func storedVersion(t *testing.T, c *caller, id string) string {
	t.Helper()

	record, err := c.stored(id)
	require.NoError(t, err)

	return store.Version(record)
}

// storedCount is every medication in the instance, both accounts. A refused
// write that stored a row for the WRONG account would be invisible to a count
// scoped to the caller, which is the point of not scoping it.
func storedCount(t *testing.T, c *caller) int {
	t.Helper()

	rows, err := c.app.FindAllRecords(kind.Medication.Collection())
	require.NoError(t, err)

	return len(rows)
}

// storedCountOf is one account's rows.
func storedCountOf(t *testing.T, c *caller, ownerID string) int {
	t.Helper()

	rows, err := c.app.FindAllRecords(kind.Medication.Collection())
	require.NoError(t, err)

	owned := 0

	for _, row := range rows {
		if row.GetString("owner") == ownerID {
			owned++
		}
	}

	return owned
}

// storeMedication reads a stored row back through the same mapper the
// repository uses, so the comparison is against the record and not against a
// second opinion of what its columns mean.
func storeMedication(record *core.Record) (clinical.Medication, error) {
	return store.MedicationFromRecord(record)
}
