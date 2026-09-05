package clinical

import "slices"

// FamilyRelationship is a relative's relation to the patient, with a
// catch-all.
type FamilyRelationship string

const (
	FamilyRelationshipMother      FamilyRelationship = "mother"
	FamilyRelationshipFather      FamilyRelationship = "father"
	FamilyRelationshipSister      FamilyRelationship = "sister"
	FamilyRelationshipBrother     FamilyRelationship = "brother"
	FamilyRelationshipDaughter    FamilyRelationship = "daughter"
	FamilyRelationshipSon         FamilyRelationship = "son"
	FamilyRelationshipGrandmother FamilyRelationship = "grandmother"
	FamilyRelationshipGrandfather FamilyRelationship = "grandfather"
	FamilyRelationshipAunt        FamilyRelationship = "aunt"
	FamilyRelationshipUncle       FamilyRelationship = "uncle"
	FamilyRelationshipCousin      FamilyRelationship = "cousin"
	FamilyRelationshipNiece       FamilyRelationship = "niece"
	FamilyRelationshipNephew      FamilyRelationship = "nephew"
	FamilyRelationshipHalfSibling FamilyRelationship = "half_sibling"
	FamilyRelationshipOther       FamilyRelationship = "other"
)

var familyRelationships = []FamilyRelationship{
	FamilyRelationshipMother, FamilyRelationshipFather, FamilyRelationshipSister,
	FamilyRelationshipBrother, FamilyRelationshipDaughter, FamilyRelationshipSon,
	FamilyRelationshipGrandmother, FamilyRelationshipGrandfather, FamilyRelationshipAunt,
	FamilyRelationshipUncle, FamilyRelationshipCousin, FamilyRelationshipNiece,
	FamilyRelationshipNephew, FamilyRelationshipHalfSibling, FamilyRelationshipOther,
}

func FamilyRelationships() []FamilyRelationship { return slices.Clone(familyRelationships) }

func (v FamilyRelationship) Valid() bool { return slices.Contains(familyRelationships, v) }
