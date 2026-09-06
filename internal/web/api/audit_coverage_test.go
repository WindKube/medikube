package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T209, FR-084, FR-085, SC-011. Every one of the fourteen registered kinds,
// driven through the real HTTP surface: a creation, a correction, a
// tags-only change and a deletion each produce an audit entry, a refused
// access attempt produces access_denied, and nothing written into any of
// those bodies' free-text fields ever shows up in the audit trail.
//
// TestCourseMedicationRelationshipChangesProduceAuditEntriesAndNoPHI below
// covers the relationship leg: attaching, re-attaching and detaching a course
// medication.

// auditKindCase is one kind's create body and the field a correction PATCH
// changes. sentinel is embedded in that field at create time and never
// appears again outside the record's own collection: internal/platform/pb's
// RecordAudit.event has no column a name, a diagnosis or a note could be
// written into, and this is the exercise that proves it rather than assumes
// it (data-model §3).
type auditKindCase struct {
	create          func(patientID, sentinel string) string
	correctionField string
}

func auditKindCases() map[kind.Kind]auditKindCase {
	return map[kind.Kind]auditKindCase{
		kind.Medication: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q}`, p, s)
			},
			correctionField: "name",
		},
		kind.Allergy: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"allergen":%q,"severity":"mild"}`, p, s)
			},
			correctionField: "allergen",
		},
		kind.Condition: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"diagnosis":%q,"status":"active"}`, p, s)
			},
			correctionField: "diagnosis",
		},
		kind.EmergencyContact: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"relationship":"spouse","phone":"+1-555-0100"}`, p, s)
			},
			correctionField: "name",
		},
		kind.Encounter: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"reason":%q,"occurred_on":"2026-01-10"}`, p, s)
			},
			correctionField: "reason",
		},
		kind.Procedure: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"occurred_on":"2026-01-10","status":"completed"}`, p, s)
			},
			correctionField: "name",
		},
		kind.Treatment: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"started_on":"2026-01-10"}`, p, s)
			},
			correctionField: "name",
		},
		kind.Symptom: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"severity":"moderate","occurred_at":"2026-01-01T09:00:00Z"}`, p, s)
			},
			correctionField: "name",
		},
		kind.Vitals: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"recorded_at":"2026-01-01T09:00:00Z","weight_kg":70,"device":%q}`, p, s)
			},
			correctionField: "device",
		},
		kind.Immunization: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"vaccine_name":%q,"administered_on":"2024-01-01"}`, p, s)
			},
			correctionField: "vaccine_name",
		},
		kind.Injury: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"body_part":"ankle"}`, p, s)
			},
			correctionField: "name",
		},
		kind.Insurance: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"type":"medical","company":%q,"member_name":"Amara Okafor","member_id":"MEM-001","effective_on":"2024-01-01"}`, p, s)
			},
			correctionField: "company",
		},
		kind.Equipment: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"type":"cpap"}`, p, s)
			},
			correctionField: "name",
		},
		kind.FamilyMember: {
			create: func(p, s string) string {
				return fmt.Sprintf(`{"patient":%q,"name":%q,"relationship":"aunt"}`, p, s)
			},
			correctionField: "name",
		},
	}
}

func TestCreateCorrectTagAndDeleteAcrossEveryKindProducesAuditEntriesAndNoPHI(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)

	cases := auditKindCases()
	require.Len(t, cases, len(kind.Kinds()), "every registered kind must have an audit case in this test")

	tag := owner.post(tagsURL(), `{"name":"audit-coverage-tag"}`)
	require.Equal(t, http.StatusCreated, tag.Status, tag.Body)
	tagID := tag.tag(t).ID

	var sentinels []string

	for _, k := range kind.Kinds() {
		kase, ok := cases[k]
		require.Truef(t, ok, "%s has no case in auditKindCases", k)

		t.Run(k.Segment(), func(t *testing.T) {
			collectionURL := "/api/v1/records/" + k.Segment()
			sentinel := "audit-phi-sentinel-" + k.Segment()
			sentinels = append(sentinels, sentinel)

			created := owner.post(collectionURL, kase.create(testsupport.AccountAPatientChildID, sentinel))
			require.Equalf(t, http.StatusCreated, created.Status, "%s: create: %s", k, created.Body)

			id := created.items1(t).ID
			etag := created.etag(t)

			assert.Truef(t, hasAuditRow(t, owner, k, "create", id),
				"%s: no create audit row for %s", k, id)

			corrected := owner.patch(collectionURL+"/"+id,
				fmt.Sprintf(`{%q:%q}`, kase.correctionField, sentinel+"-corrected"), etag)
			require.Equalf(t, http.StatusOK, corrected.Status, "%s: correction: %s", k, corrected.Body)
			etag = corrected.etag(t)

			assert.Truef(t, hasAuditRow(t, owner, k, "update", id),
				"%s: no update audit row after correcting %s", k, id)

			tagged := owner.patch(collectionURL+"/"+id, fmt.Sprintf(`{"tags":[%q]}`, tagID), etag)
			require.Equalf(t, http.StatusOK, tagged.Status, "%s: tag change: %s", k, tagged.Body)
			etag = tagged.etag(t)

			assert.GreaterOrEqualf(t, countAuditRows(t, owner, k, "update", id), 2,
				"%s: a tags-only change to %s did not produce its own audit entry", k, id)

			deniedBefore := countAuditRows(t, owner, "", "access_denied", "")

			refused := stranger.get(collectionURL + "/" + id)
			assert.Equal(t, http.StatusNotFound, refused.Status, "%s: a stranger must be refused", k)

			assert.Greaterf(t, countAuditRows(t, owner, "", "access_denied", ""), deniedBefore,
				"%s: the stranger's refused access produced no access_denied entry", k)

			removed := owner.delete(collectionURL+"/"+id, etag)
			require.Equalf(t, http.StatusNoContent, removed.Status, "%s: delete: %s", k, removed.Body)

			assert.Truef(t, hasAuditRow(t, owner, k, "delete", id),
				"%s: no delete audit row for %s", k, id)
		})
	}

	// The PHI sweep: every sentinel embedded in a kind's own free-text field
	// at create time, checked against every column of every audit_events row.
	blob := auditEventsBlob(t, owner)

	for _, sentinel := range sentinels {
		assert.NotContainsf(t, blob, sentinel,
			"a sentinel written into a record's own field reached the audit trail: %s", sentinel)
	}
}

// hasAuditRow and countAuditRows read the audit trail straight out of the
// database rather than through any API, so the assertion is about what was
// written and not about what a read endpoint chooses to shape.
func hasAuditRow(t *testing.T, c *caller, k kind.Kind, action, targetID string) bool {
	t.Helper()

	return countAuditRows(t, c, k, action, targetID) > 0
}

func countAuditRows(t *testing.T, c *caller, k kind.Kind, action, targetID string) int {
	t.Helper()

	rows, err := c.app.FindAllRecords("audit_events")
	require.NoError(t, err)

	n := 0

	for _, row := range rows {
		if row.GetString("action") != action {
			continue
		}

		if k != "" && row.GetString("target_kind") != k.Enum() {
			continue
		}

		if targetID != "" && row.GetString("target_id") != targetID {
			continue
		}

		n++
	}

	return n
}

// auditEventsBlob concatenates every string-shaped column of every
// audit_events row, so a single Contains check covers the whole table.
func auditEventsBlob(t *testing.T, c *caller) string {
	t.Helper()

	rows, err := c.app.FindAllRecords("audit_events")
	require.NoError(t, err)

	var b strings.Builder

	for _, row := range rows {
		for _, field := range []string{"actor", "actor_kind", "action", "target_kind", "target_id", "request_id", "patient"} {
			b.WriteString(row.GetString(field))
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// TestCourseMedicationRelationshipChangesProduceAuditEntriesAndNoPHI is
// T209's relationship leg, FR-084 ("every relationship created or removed")
// and FR-085 applied to the one payload-carrying join US6 adds:
// treatment_medications is not a kind.Kind, so attaching, re-attaching and
// detaching a course medication is not seen by the generic per-kind audit
// hooks and must write its own row — carrying no dosage, frequency, timing,
// prescriber or pharmacy.
func TestCourseMedicationRelationshipChangesProduceAuditEntriesAndNoPHI(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	treatment := owner.post("/api/v1/records/"+kind.Treatment.Segment(),
		fmt.Sprintf(`{"patient":%q,"name":"Course medication audit","started_on":"2026-01-10"}`,
			testsupport.AccountAPatientChildID))
	require.Equal(t, http.StatusCreated, treatment.Status, treatment.Body)
	treatmentID := treatment.items1(t).ID

	medication := owner.post("/api/v1/records/"+kind.Medication.Segment(),
		fmt.Sprintf(`{"patient":%q,"name":"Course medication audit"}`, testsupport.AccountAPatientChildID))
	require.Equal(t, http.StatusCreated, medication.Status, medication.Body)
	medicationID := medication.items1(t).ID

	current := owner.get(treatmentRecordURL(treatmentID))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	dosageSentinel := "audit-phi-sentinel-course-medication-dosage"

	createsBefore := countAuditRows(t, owner, kind.Treatment, "create", treatmentID)

	attached := owner.do(http.MethodPut, courseMedicationItemURL(treatmentID, medicationID),
		fmt.Sprintf(`{"dosage":%q}`, dosageSentinel), map[string]string{"If-Match": current.etag(t)})
	require.Equal(t, http.StatusCreated, attached.Status, attached.Body)

	assert.Greaterf(t, countAuditRows(t, owner, kind.Treatment, "create", treatmentID), createsBefore,
		"attaching a course medication for the first time must audit as its own creation")

	updatesBefore := countAuditRows(t, owner, kind.Treatment, "update", treatmentID)

	reattached := owner.do(http.MethodPut, courseMedicationItemURL(treatmentID, medicationID),
		fmt.Sprintf(`{"dosage":%q}`, dosageSentinel), map[string]string{"If-Match": current.etag(t)})
	require.Equal(t, http.StatusOK, reattached.Status, reattached.Body)

	assert.Greaterf(t, countAuditRows(t, owner, kind.Treatment, "update", treatmentID), updatesBefore,
		"re-attaching the same pair must audit as an update, not a second creation")

	deletesBefore := countAuditRows(t, owner, kind.Treatment, "delete", treatmentID)

	detached := owner.delete(courseMedicationItemURL(treatmentID, medicationID), current.etag(t))
	require.Equal(t, http.StatusNoContent, detached.Status, detached.Body)

	assert.Greaterf(t, countAuditRows(t, owner, kind.Treatment, "delete", treatmentID), deletesBefore,
		"detaching a course medication must audit as a deletion")

	blob := auditEventsBlob(t, owner)
	assert.NotContainsf(t, blob, dosageSentinel,
		"a course medication's own dosage must never reach the audit trail: %s", dosageSentinel)
}
