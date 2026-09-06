package search_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/search"
)

// lifecycleService is a minimal records.Service, standing in for
// recordstest's own FakeKindService only because that one mints ids shorter
// than search_index's 15-character record_id column — a detail that has
// never mattered until something actually writes the row it mints, which is
// exactly what this test does. Everything it needs beyond an id — a name it
// can turn into a title through recordstest.SearchFields — it produces the
// same shape recordstest.Detail does, so the registration underneath it is
// otherwise identical to recordstest.Registration's own.
type lifecycleService struct {
	n    int
	rows map[string]lifecycleRow
}

type lifecycleRow struct {
	patient string
	version int
	name    string
	tags    []string
}

// lifecycleCreate and lifecyclePatch stand in for recordstest.Create/Patch,
// with a Tags member neither of those carries: this file's own service reads
// them directly rather than through recordstest.Schema, so nothing but this
// test needs recordstest itself to grow a tags field it has no other use for.
type lifecycleCreate struct {
	Name string
	Tags []string
}

type lifecyclePatch struct {
	Name *string
	Tags *[]string
}

func newLifecycleService() *lifecycleService {
	return &lifecycleService{rows: map[string]lifecycleRow{}}
}

func (s *lifecycleService) List(context.Context, access.Actor, records.Query) (domain.Page[records.Record], error) {
	return domain.Page[records.Record]{}, nil
}

func (s *lifecycleService) Get(_ context.Context, _ access.Actor, id string) (records.Record, error) {
	row, ok := s.rows[id]
	if !ok {
		return records.Record{}, domain.ErrNotFound
	}

	return s.record(id, row), nil
}

func (s *lifecycleService) Create(_ context.Context, actor access.Actor, body any) (records.Record, error) {
	create := body.(*lifecycleCreate) //nolint:forcetypeassert // this fake mints exactly what it declares

	s.n++
	id := recordID(s.n)
	row := lifecycleRow{patient: actor.UserID, version: 1, name: create.Name, tags: create.Tags}
	s.rows[id] = row

	return s.record(id, row), nil
}

func (s *lifecycleService) Update(_ context.Context, actor access.Actor, id, version string, body any) (records.Record, error) {
	row, ok := s.rows[id]
	if !ok || row.patient != actor.UserID {
		return records.Record{}, domain.ErrNotFound
	}

	if version != fmt.Sprint(row.version) {
		return records.Record{}, domain.ErrVersionMismatch
	}

	patch := body.(*lifecyclePatch) //nolint:forcetypeassert // as above

	if patch.Name != nil {
		row.name = *patch.Name
	}

	if patch.Tags != nil {
		row.tags = *patch.Tags
	}

	row.version++
	s.rows[id] = row

	return s.record(id, row), nil
}

func (s *lifecycleService) Delete(_ context.Context, actor access.Actor, id, version string) error {
	row, ok := s.rows[id]
	if !ok || row.patient != actor.UserID {
		return domain.ErrNotFound
	}

	if version != fmt.Sprint(row.version) {
		return domain.ErrVersionMismatch
	}

	delete(s.rows, id)

	return nil
}

func (s *lifecycleService) record(id string, row lifecycleRow) records.Record {
	detail := recordstest.Detail{
		Summary: recordstest.Summary{ID: id, Kind: string(kind.Medication), Name: row.name},
		Tags:    row.tags,
	}

	return records.Record{
		ID: id, Kind: kind.Medication, PatientID: row.patient, Version: fmt.Sprint(row.version), Body: &detail,
	}
}

// TestIndexLifecycle is T169: one registered kind, wired the way
// records.Registry actually wires it (SetIndexer, then Register), so what is
// under test is the whole path a real create/update/delete takes — not just
// the Indexer in isolation (already covered by
// internal/service/search/indexer_test.go's fakes).
func TestIndexLifecycle(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()

	indexer, err := search.NewIndexer(h.repo)
	require.NoError(t, err)

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Service = newLifecycleService()

	registry := records.NewRegistry()
	registry.SetIndexer(indexer)
	require.NoError(t, registry.Register(registration))

	entry, registered := registry.FromKind(kind.Medication)
	require.True(t, registered)

	actor := access.Actor{UserID: h.patient}

	chronic := seedTag(t, h.app, h.owner, "chronic")
	reviewed := seedTag(t, h.app, h.owner, "reviewed")

	created, createErr := entry.Service.Create(ctx, actor, &lifecycleCreate{Name: "Warfarin", Tags: []string{chronic}})
	require.NoError(t, createErr)

	row, found, findErr := h.repo.Find(ctx, kind.Medication, created.ID)
	require.NoError(t, findErr)
	require.True(t, found, "creating a record must write exactly one index row")
	assert.Equal(t, "Warfarin", row.Title)
	assert.Equal(t, []string{chronic}, row.TagIDs, "a create must seed search_index.tags from the record's own tags")

	updatedTags := []string{chronic, reviewed}
	updatedRecord, updateErr := entry.Service.Update(ctx, actor, created.ID, created.Version,
		&lifecyclePatch{Name: strPtr("Warfarin XR"), Tags: &updatedTags})
	require.NoError(t, updateErr)

	updatedRow, foundAfter, findAfterErr := h.repo.Find(ctx, kind.Medication, created.ID)
	require.NoError(t, findAfterErr)
	require.True(t, foundAfter)
	assert.Equal(t, "Warfarin XR", updatedRow.Title, "an update must replace the row's title")
	assert.ElementsMatch(t, updatedTags, updatedRow.TagIDs, "an update must keep search_index.tags in step with the record's own tags")

	page, pageErr := h.repo.Page(ctx, h.patient, []kind.Kind{kind.Medication}, 10, "")
	require.NoError(t, pageErr)
	assert.Len(t, page.Items, 1, "an update must replace the one row, never add a second")

	require.NoError(t, entry.Service.Delete(ctx, actor, created.ID, updatedRecord.Version))

	_, foundAfterDelete, findErr := h.repo.Find(ctx, kind.Medication, created.ID)
	require.NoError(t, findErr)
	assert.False(t, foundAfterDelete, "deleting a record must remove its index row")
}

// TestDeletePatientRemovesEveryRow is FR-087/SC-005: cascading a patient's
// delete removes every kind's row for that patient, not just one.
func TestDeletePatientRemovesEveryRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	h.seedRow(t, kind.Medication, recordID(1), &on)
	h.seedRow(t, kind.Allergy, recordID(2), &on)

	require.NoError(t, h.repo.RemoveByPatient(ctx, h.patient))

	total, err := h.repo.Count(ctx, h.patient, nil)
	require.NoError(t, err)
	assert.Zero(t, total)
}

func strPtr(s string) *string { return &s }
