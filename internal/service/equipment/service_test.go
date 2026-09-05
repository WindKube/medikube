package equipment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/equipment"
	"medikube/internal/service/equipment/equipmenttest"
)

func actor() access.Actor {
	return access.Actor{UserID: equipmenttest.OwnerID, RequestID: "req-equipment"}
}

func stranger() access.Actor {
	return access.Actor{UserID: equipmenttest.StrangerID, RequestID: "req-equipment-stranger"}
}

func newService(t *testing.T) (*equipment.Service, *equipmenttest.Repository) {
	t.Helper()

	repository := equipmenttest.NewRepository()
	authorizer := equipmenttest.NewAuthorizer(equipmenttest.OwnerID)

	service, err := equipment.New(repository, authorizer)
	require.NoError(t, err)

	return service, repository
}

func TestServiceNew(t *testing.T) {
	t.Parallel()

	authorizer := equipmenttest.NewAuthorizer(equipmenttest.OwnerID)

	_, err := equipment.New(nil, authorizer)
	assert.Error(t, err)

	_, err = equipment.New(equipmenttest.NewRepository(), nil)
	assert.Error(t, err)
}

func TestCreateRequiresAPatient(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	_, err := service.Create(t.Context(), actor(), clinical.Equipment{Name: "CPAP", Type: clinical.EquipmentTypeCPAP})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, equipment.FieldPatient, invalid.Fields[0].Field)
}

func TestCreateRefusesAnInvalidDraft(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	_, err := service.Create(t.Context(), actor(), clinical.Equipment{PatientID: equipmenttest.PatientID})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestCreateGet(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	created, err := service.Create(t.Context(), actor(), clinical.Equipment{
		PatientID: equipmenttest.PatientID, Name: "CPAP machine", Type: clinical.EquipmentTypeCPAP,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	found, err := service.Get(t.Context(), actor(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)

	_, err = service.Get(t.Context(), stranger(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpdateAndDeleteCheckVersion(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	created, err := service.Create(t.Context(), actor(), clinical.Equipment{
		PatientID: equipmenttest.PatientID, Name: "CPAP machine", Type: clinical.EquipmentTypeCPAP,
	})
	require.NoError(t, err)

	_, err = service.Update(t.Context(), actor(), created.ID, "stale", equipment.Patch{})
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	newName := "New CPAP machine"
	updated, err := service.Update(t.Context(), actor(), created.ID, created.Version, equipment.Patch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, newName, updated.Name)

	err = service.Delete(t.Context(), actor(), created.ID, "stale")
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	require.NoError(t, service.Delete(t.Context(), actor(), created.ID, updated.Version))

	_, err = service.Get(t.Context(), actor(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListRefusesAnUnpublishedFilter(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	_, err := service.List(t.Context(), actor(), equipment.Query{
		PatientID: equipmenttest.PatientID,
		Types:     []clinical.EquipmentType{"spaceship"},
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestServiceDueBasis(t *testing.T) {
	t.Parallel()

	today := clinical.Today()

	past, err := domain.ParseDate("2020-01-01")
	require.NoError(t, err)

	far, err := domain.ParseDate("2099-01-01")
	require.NoError(t, err)

	cases := []struct {
		name   string
		dueOn  clinical.Date
		within int
		want   []string
	}{
		{name: "no due date at all", dueOn: clinical.Date{}, within: 30, want: nil},
		{name: "overdue", dueOn: past, within: 30, want: []string{equipment.BasisOverdue}},
		{name: "due soon", dueOn: today, within: 30, want: []string{equipment.BasisDueSoon}},
		{name: "beyond the horizon", dueOn: far, within: 30, want: nil},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := equipment.ServiceDueBasis(clinical.Equipment{ServiceDueOn: tt.dueOn}, tt.within)
			assert.Equal(t, tt.want, got)
		})
	}
}
