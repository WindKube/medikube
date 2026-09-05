package person

import "slices"

// Sex collapses upstream's seven aliases (M, F, MALE, FEMALE, OTHER, U,
// UNKNOWN) into four concepts, per SHARED-DESIGN §1.2.
type Sex string

const (
	SexFemale      Sex = "female"
	SexMale        Sex = "male"
	SexIntersex    Sex = "intersex"
	SexUnspecified Sex = "unspecified"
)

// BloodType is the ABO/Rh vocabulary, closed rather than free text.
type BloodType string

const (
	BloodTypeAPos  BloodType = "a_pos"
	BloodTypeANeg  BloodType = "a_neg"
	BloodTypeBPos  BloodType = "b_pos"
	BloodTypeBNeg  BloodType = "b_neg"
	BloodTypeABPos BloodType = "ab_pos"
	BloodTypeABNeg BloodType = "ab_neg"
	BloodTypeOPos  BloodType = "o_pos"
	BloodTypeONeg  BloodType = "o_neg"
)

// RelationshipToOwner is the account holder's relationship to the person the
// profile describes. FR-001.
type RelationshipToOwner string

const (
	RelationshipSelf    RelationshipToOwner = "self"
	RelationshipSpouse  RelationshipToOwner = "spouse"
	RelationshipPartner RelationshipToOwner = "partner"
	RelationshipParent  RelationshipToOwner = "parent"
	RelationshipChild   RelationshipToOwner = "child"
	RelationshipSibling RelationshipToOwner = "sibling"
	RelationshipWard    RelationshipToOwner = "ward"
	RelationshipOther   RelationshipToOwner = "other"
)

// One declaration per vocabulary, in the order the form offers it. Valid() and
// the accessor read the same slice, so a value cannot be accepted without being
// offered or offered without being accepted.
var (
	sexes      = []Sex{SexFemale, SexMale, SexIntersex, SexUnspecified}
	bloodTypes = []BloodType{
		BloodTypeAPos, BloodTypeANeg,
		BloodTypeBPos, BloodTypeBNeg,
		BloodTypeABPos, BloodTypeABNeg,
		BloodTypeOPos, BloodTypeONeg,
	}
	relationshipsToOwner = []RelationshipToOwner{
		RelationshipSelf, RelationshipSpouse, RelationshipPartner, RelationshipParent,
		RelationshipChild, RelationshipSibling, RelationshipWard, RelationshipOther,
	}
)

// Sexes is the published vocabulary, in the order the form offers it, and so
// are BloodTypes and RelationshipsToOwner below. They clone, as every such
// accessor in this codebase does, because a caller that sorted the result for
// one display would otherwise reorder every form, every OpenAPI enum and
// every Valid() along with it.
func Sexes() []Sex                                { return slices.Clone(sexes) }
func BloodTypes() []BloodType                     { return slices.Clone(bloodTypes) }
func RelationshipsToOwner() []RelationshipToOwner { return slices.Clone(relationshipsToOwner) }

// Valid is false for the empty string on all three. All three fields are
// optional, so an empty value is absence and not a rejected value; Validate is
// what tells the two apart.
func (s Sex) Valid() bool                 { return slices.Contains(sexes, s) }
func (b BloodType) Valid() bool           { return slices.Contains(bloodTypes, b) }
func (r RelationshipToOwner) Valid() bool { return slices.Contains(relationshipsToOwner, r) }
