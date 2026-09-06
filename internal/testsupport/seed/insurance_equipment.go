package seed

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// daysFromNow is insurance's expiry and equipment's service-due dates, which
// name a distance from today (FR-046, FR-049) rather than a fixed calendar
// day: a fixture pinned to a literal date would stop qualifying as "expiring
// soon" the moment the calendar caught up with it.
func daysFromNow(days int) domain.Date {
	value, err := domain.NewDate(time.Now().UTC().AddDate(0, 0, days).Date())
	if err != nil {
		panic(fmt.Sprintf("seed: today plus %d days is not a calendar date", days))
	}

	return value
}

// InsurancePrimaryID and InsuranceExpiringID are data-model §4.10's two
// mandatory fixture rows (US5): one primary policy, and one expiring within
// forty-five days.
const (
	InsurancePrimaryID  = "mkinsamara00001"
	InsuranceExpiringID = "mkinsamara00002"
)

// Insurances is account A's two policies. Account B and account A's second
// patient carry none, the same "an empty list is a real state" reasoning
// Medications documents above.
func Insurances() []clinical.Insurance {
	return []clinical.Insurance{
		{
			ID: InsurancePrimaryID, PatientID: accountAPatientSelfID,
			Type: clinical.InsuranceTypeMedical, Company: "Meridian Health Assurance",
			PlanName: "PPO Gold", MemberName: "Amara Okonkwo", MemberID: "MHA-778142",
			EffectiveOn: date(2024, 1, 1), ExpiresOn: date(2099, 1, 1),
			Status: clinical.InsuranceStatusActive, IsPrimary: true,
		},
		{
			ID: InsuranceExpiringID, PatientID: accountAPatientParentID,
			Type: clinical.InsuranceTypeDental, Company: "Riverbend Dental Trust",
			MemberName: "Amara Okonkwo", MemberID: "RDT-224917",
			EffectiveOn: date(2025, 1, 1), ExpiresOn: daysFromNow(45),
			Status: clinical.InsuranceStatusActive,
		},
	}
}

// EquipmentOverdueID and EquipmentDueSoonID are US5's two mandatory equipment
// rows: one overdue for service, one due within twenty days.
const (
	EquipmentOverdueID = "mkeqpamara00001"
	EquipmentDueSoonID = "mkeqpamara00002"
)

// Equipment is account A's self-record's two rows. Account A's second patient
// carries none (US5's "/equipment left empty for one seeded patient"), and
// account B carries none either.
func Equipment() []clinical.Equipment {
	return []clinical.Equipment{
		{
			ID: EquipmentOverdueID, PatientID: accountAPatientSelfID,
			Name: "CPAP machine", Type: clinical.EquipmentTypeCPAP,
			Manufacturer: "ResMed", Model: "AirSense 11",
			ServicedOn: date(2024, 1, 15), ServiceDueOn: daysFromNow(-30),
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: EquipmentDueSoonID, PatientID: accountAPatientSelfID,
			Name: "Nebulizer", Type: clinical.EquipmentTypeNebulizer,
			Manufacturer: "Omron", Model: "NE-C801",
			ServicedOn: date(2025, 1, 1), ServiceDueOn: daysFromNow(20),
			Status: clinical.TherapyStatusActive,
		},
	}
}

const (
	columnCompany     = "company"
	columnMemberName  = "member_name"
	columnMemberID    = "member_id"
	columnPlanName    = "plan_name"
	columnEffectiveOn = "effective_on"
	columnIsPrimary   = "is_primary"
)

const (
	columnModel        = "model"
	columnServicedOn   = "serviced_on"
	columnServiceDueOn = "service_due_on"
)

func applyInsurances(app core.App) error {
	name := kind.Insurance.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnType, columnCompany, columnPlanName, columnMemberName,
		columnMemberID, columnEffectiveOn, columnExpiresOn, columnStatus, columnIsPrimary,
	); err != nil {
		return err
	}

	for _, insurance := range Insurances() {
		if err := insurance.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", insurance.ID, err)
		}

		record, err := findOrNew(app, collection, insurance.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, insurance.PatientID)
		record.Set(columnType, string(insurance.Type))
		record.Set(columnCompany, insurance.Company)
		record.Set(columnPlanName, insurance.PlanName)
		record.Set(columnMemberName, insurance.MemberName)
		record.Set(columnMemberID, insurance.MemberID)
		record.Set(columnEffectiveOn, insurance.EffectiveOn.UTC())
		record.Set(columnExpiresOn, insurance.ExpiresOn.UTC())
		record.Set(columnStatus, string(insurance.Status))
		record.Set(columnIsPrimary, insurance.IsPrimary)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", insurance.ID, err)
		}

		if err := IndexRecord(app, kind.Insurance, insurance.ID, insurance.PatientID,
			insurance.Company, insurance.PlanName, insurance.EffectiveOn); err != nil {
			return err
		}
	}

	return nil
}

func applyEquipment(app core.App) error {
	name := kind.Equipment.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnType, columnManufacturer, columnModel,
		columnServicedOn, columnServiceDueOn, columnStatus,
	); err != nil {
		return err
	}

	for _, item := range Equipment() {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", item.ID, err)
		}

		record, err := findOrNew(app, collection, item.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, item.PatientID)
		record.Set(columnName, item.Name)
		record.Set(columnType, string(item.Type))
		record.Set(columnManufacturer, item.Manufacturer)
		record.Set(columnModel, item.Model)
		record.Set(columnServicedOn, item.ServicedOn.UTC())
		record.Set(columnServiceDueOn, item.ServiceDueOn.UTC())
		record.Set(columnStatus, string(item.Status))

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", item.ID, err)
		}

		if err := IndexRecord(app, kind.Equipment, item.ID, item.PatientID,
			item.Name, item.Instructions, item.ServiceDueOn); err != nil {
			return err
		}
	}

	return nil
}
