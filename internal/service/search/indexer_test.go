package search

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

type fakeRepository struct {
	rows map[string]Row // keyed by kind+recordID
}

func rowKey(k kind.Kind, recordID string) string {
	return string(k) + "/" + recordID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[string]Row{}}
}

func (f *fakeRepository) Upsert(_ context.Context, row Row) error {
	f.rows[rowKey(row.Kind, row.RecordID)] = row
	return nil
}

func (f *fakeRepository) Remove(_ context.Context, k kind.Kind, recordID string) error {
	delete(f.rows, rowKey(k, recordID))
	return nil
}

func (f *fakeRepository) RemoveByPatient(_ context.Context, patientID string) error {
	for key, row := range f.rows {
		if row.PatientID == patientID {
			delete(f.rows, key)
		}
	}
	return nil
}

func TestNewIndexerRefusesANilRepository(t *testing.T) {
	ix, err := NewIndexer(nil)
	require.ErrorIs(t, err, ErrNoRepository)
	assert.Nil(t, ix)
}

func TestCreateUpsertsExactlyOneRow(t *testing.T) {
	repo := newFakeRepository()
	ix, err := NewIndexer(repo)
	require.NoError(t, err)

	row := Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec1", Title: "Amoxicillin"}
	require.NoError(t, ix.Create(context.Background(), row))

	assert.Len(t, repo.rows, 1)
	assert.Equal(t, row, repo.rows[rowKey(kind.Medication, "rec1")])
}

func TestUpdateReplacesTheExistingRow(t *testing.T) {
	repo := newFakeRepository()
	ix, err := NewIndexer(repo)
	require.NoError(t, err)

	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec1", Title: "Amoxicillin"}))
	require.NoError(t, ix.Update(context.Background(), Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec1", Title: "Amoxicillin 500mg"}))

	require.Len(t, repo.rows, 1)
	assert.Equal(t, "Amoxicillin 500mg", repo.rows[rowKey(kind.Medication, "rec1")].Title)
}

func TestDeleteRemovesOnlyItsOwnRow(t *testing.T) {
	repo := newFakeRepository()
	ix, err := NewIndexer(repo)
	require.NoError(t, err)

	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec1"}))
	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec2"}))

	require.NoError(t, ix.Delete(context.Background(), kind.Medication, "rec1"))

	assert.Len(t, repo.rows, 1)
	_, stillThere := repo.rows[rowKey(kind.Medication, "rec2")]
	assert.True(t, stillThere)
}

func TestDeletePatientCascadesEveryRowAway(t *testing.T) {
	repo := newFakeRepository()
	ix, err := NewIndexer(repo)
	require.NoError(t, err)

	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat1", Kind: kind.Medication, RecordID: "rec1"}))
	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat1", Kind: kind.Procedure, RecordID: "rec2"}))
	require.NoError(t, ix.Create(context.Background(), Row{PatientID: "pat2", Kind: kind.Medication, RecordID: "rec3"}))

	require.NoError(t, ix.DeletePatient(context.Background(), "pat1"))

	require.Len(t, repo.rows, 1)
	_, stillThere := repo.rows[rowKey(kind.Medication, "rec3")]
	assert.True(t, stillThere)
}
