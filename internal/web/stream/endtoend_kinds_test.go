package stream_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/views/ids"
)

// Every kind the keyboard gate records reaches a second open view as a
// prepended row, with the body the browser form actually submits: an empty
// number control is 0 on the wire.
func TestACreateOfEveryKindIsPrependedIntoItsList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind kind.Kind
		body string
	}{
		{kind.Condition, `{"diagnosis":"E2E condition","status":"active","severity":"","onset_on":"","resolved_on":"","icd10_code":"","snomed_code":"","notes":""}`},
		{kind.Encounter, `{"reason":"E2E encounter","occurred_on":"2025-06-01","visit_type":"","priority":"","assessment":"","plan":"","follow_up":"","duration_minutes":0,"notes":""}`},
		{kind.Insurance, `{"company":"E2E insurer","type":"medical","plan_name":"","employer_group":"","member_name":"Keyboard","member_id":"KB-1","group_number":"","holder_name":"","relationship_to_holder":"self","effective_on":"2025-01-01","expires_on":"","status":"active","is_primary":false,"notes":""}`},
		{kind.FamilyMember, `{"name":"E2E relative","relationship":"mother","sex":"","birth_year":0,"death_year":0,"is_deceased":false}`},
		{kind.Immunization, `{"vaccine_name":"E2E vaccination","trade_name":"","administered_on":"2025-01-01","dose_number":0,"lot_number":"","manufacturer":"","site":"","route":"","expires_on":""}`},
		{kind.Vitals, `{"recorded_at":"2025-06-01T07:00:00Z","heart_rate_bpm":72,"device":"E2E device"}`},
	}

	medikube := serve(t, fastHeartbeat())
	amara := medikube.token(t, testsupport.AccountAEmail)

	for _, one := range cases {
		t.Run(string(one.kind), func(t *testing.T) {
			watching := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
			require.Equal(t, http.StatusOK, watching.Response.StatusCode)

			body := `{"patient":"` + testsupport.AccountAPatientSelfID + `",` + one.body[1:]
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
				medikube.base+"/api/v1/records/"+one.kind.Segment(), strings.NewReader(body))
			require.NoError(t, err)
			request.Header.Set("Authorization", amara)
			request.Header.Set("Content-Type", "application/json")

			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			raw, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			require.Equalf(t, http.StatusCreated, response.StatusCode, "%s", raw)

			seen := watching.nextPatch(sc007)
			assert.Equal(t, "#"+ids.RecordRows(one.kind), seen.selector())
			assert.Equal(t, "prepend", seen.mode())
			assert.Contains(t, seen.elements(), `<tr`)
		})
	}
}
