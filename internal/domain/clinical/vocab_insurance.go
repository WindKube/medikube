package clinical

import "slices"

// InsuranceType is what the policy covers, with a catch-all.
type InsuranceType string

const (
	InsuranceTypeMedical      InsuranceType = "medical"
	InsuranceTypeDental       InsuranceType = "dental"
	InsuranceTypeVision       InsuranceType = "vision"
	InsuranceTypePrescription InsuranceType = "prescription"
	InsuranceTypeOther        InsuranceType = "other"
)

// InsuranceStatus is whether the policy is in force.
type InsuranceStatus string

const (
	InsuranceStatusActive   InsuranceStatus = "active"
	InsuranceStatusInactive InsuranceStatus = "inactive"
	InsuranceStatusExpired  InsuranceStatus = "expired"
	InsuranceStatusPending  InsuranceStatus = "pending"
)

// HolderRelationship is the patient's relation to the policyholder, with a
// catch-all.
type HolderRelationship string

const (
	HolderRelationshipSelf      HolderRelationship = "self"
	HolderRelationshipSpouse    HolderRelationship = "spouse"
	HolderRelationshipChild     HolderRelationship = "child"
	HolderRelationshipDependent HolderRelationship = "dependent"
	HolderRelationshipOther     HolderRelationship = "other"
)

var (
	insuranceTypes = []InsuranceType{
		InsuranceTypeMedical, InsuranceTypeDental, InsuranceTypeVision,
		InsuranceTypePrescription, InsuranceTypeOther,
	}

	insuranceStatuses = []InsuranceStatus{
		InsuranceStatusActive, InsuranceStatusInactive, InsuranceStatusExpired, InsuranceStatusPending,
	}

	holderRelationships = []HolderRelationship{
		HolderRelationshipSelf, HolderRelationshipSpouse, HolderRelationshipChild,
		HolderRelationshipDependent, HolderRelationshipOther,
	}
)

func InsuranceTypes() []InsuranceType           { return slices.Clone(insuranceTypes) }
func InsuranceStatuses() []InsuranceStatus      { return slices.Clone(insuranceStatuses) }
func HolderRelationships() []HolderRelationship { return slices.Clone(holderRelationships) }

func (v InsuranceType) Valid() bool      { return slices.Contains(insuranceTypes, v) }
func (v InsuranceStatus) Valid() bool    { return slices.Contains(insuranceStatuses, v) }
func (v HolderRelationship) Valid() bool { return slices.Contains(holderRelationships, v) }
