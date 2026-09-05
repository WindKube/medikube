package audit

import "slices"

// ActorKind is what kind of actor did it. It is carried beside the actor
// relation rather than derived from it because the relation does not cascade:
// when an account is deleted the reference is unset and the row survives, and
// this field is what still says a person did the thing (research D-22).
type ActorKind string

const (
	ActorKindUser      ActorKind = "user"
	ActorKindAdmin     ActorKind = "admin"
	ActorKindSuperuser ActorKind = "superuser"
	ActorKindSystem    ActorKind = "system"
)

// Action is what was done. The vocabulary is declared complete — twenty values
// — rather than grown phase by phase: a select field refuses an undeclared
// value, so a vocabulary assembled from six deltas fails in production on the
// first share, the first photo fetch and the first backup rather than in a test
// (data-model §3).
type Action string

const (
	// Written by this phase.
	ActionCreate         Action = "create"
	ActionUpdate         Action = "update"
	ActionDelete         Action = "delete"
	ActionAccessDenied   Action = "access_denied"
	ActionLogin          Action = "login"
	ActionLoginFailed    Action = "login_failed"
	ActionLogout         Action = "logout"
	ActionPasswordChange Action = "password_change"
	ActionAccountDelete  Action = "account_delete"
	ActionAdminSession   Action = "admin_session"

	// Declared here, first written later: 002 and onwards for the sensitive
	// read, 005 for sharing and invitations, 006 for exports and backups.
	// email_change is written by no phase in 001–006 and is declared anyway,
	// because PocketBase's native email-change endpoints stay reachable and the
	// day a hook audits them the value has to already exist.
	ActionReadSensitive Action = "read_sensitive"
	ActionShareGrant    Action = "share_grant"
	ActionShareRevoke   Action = "share_revoke"
	ActionShareExpire   Action = "share_expire"
	ActionInviteSend    Action = "invite_send"
	ActionInviteRespond Action = "invite_respond"
	ActionExport        Action = "export"
	ActionBackupCreate  Action = "backup_create"
	ActionBackupRestore Action = "backup_restore"
	ActionEmailChange   Action = "email_change"

	// ActionSwitchPatient is phase 002's own: "every change to the person in
	// view" (FR-020, FR-045), written when the active-patient pointer changes.
	ActionSwitchPatient Action = "switch_patient"
)

// TargetKind is what the action concerned: one of the fifteen record kinds or
// one of the eight platform kinds. Declared complete for the same reason Action
// is. The record-kind spellings are kind.Kind's spellings, and
// audit's enum test asserts every declared kind has a target here.
type TargetKind string

const (
	TargetKindMedication       TargetKind = "medication"
	TargetKindAllergy          TargetKind = "allergy"
	TargetKindCondition        TargetKind = "condition"
	TargetKindEncounter        TargetKind = "encounter"
	TargetKindProcedure        TargetKind = "procedure"
	TargetKindTreatment        TargetKind = "treatment"
	TargetKindSymptom          TargetKind = "symptom"
	TargetKindVitals           TargetKind = "vitals"
	TargetKindImmunization     TargetKind = "immunization"
	TargetKindInjury           TargetKind = "injury"
	TargetKindInsurance        TargetKind = "insurance"
	TargetKindEquipment        TargetKind = "equipment"
	TargetKindEmergencyContact TargetKind = "emergency_contact"
	TargetKindFamilyMember     TargetKind = "family_member"
	TargetKindLabResult        TargetKind = "lab_result"

	TargetKindPatient    TargetKind = "patient"
	TargetKindUser       TargetKind = "user"
	TargetKindShare      TargetKind = "share"
	TargetKindInvitation TargetKind = "invitation"
	TargetKindAttachment TargetKind = "attachment"
	TargetKindExport     TargetKind = "export"
	TargetKindBackup     TargetKind = "backup"
	TargetKindSystem     TargetKind = "system"

	// TargetKindPractitioner and TargetKindFacility are phase 002's own
	// directory kinds. Neither is one of kind.Kind's clinical record kinds —
	// the directory is not a record kind, it is what a record's practitioner
	// or pharmacy field points at.
	TargetKindPractitioner TargetKind = "practitioner"
	TargetKindFacility     TargetKind = "facility"

	// TargetKindTag and TargetKindSearch are phase 003's additive extension
	// (data-model §5.4, research D-19): a tag is an auditable resource, and
	// search is the target kind [D-12](../../../specs/003-clinical-records/research.md#d-12--fr-075-the-search-term-is-a-first-class-secret)
	// requires for the row a search writes. Written by
	// internal/store/migrations' audit_vocab migration, never by this phase's
	// record-kind migrations.
	TargetKindTag    TargetKind = "tag"
	TargetKindSearch TargetKind = "search"
)

// One declaration per vocabulary, in the order data-model §3 declares it, which
// is the order the migration writes into the select field. Valid() and the
// accessor read the same slice, so a value cannot be accepted without being
// declared or declared without being accepted.
var (
	actorKinds = []ActorKind{
		ActorKindUser,
		ActorKindAdmin,
		ActorKindSuperuser,
		ActorKindSystem,
	}

	actions = []Action{
		ActionCreate,
		ActionUpdate,
		ActionDelete,
		ActionAccessDenied,
		ActionLogin,
		ActionLoginFailed,
		ActionLogout,
		ActionPasswordChange,
		ActionAccountDelete,
		ActionAdminSession,
		ActionReadSensitive,
		ActionShareGrant,
		ActionShareRevoke,
		ActionShareExpire,
		ActionInviteSend,
		ActionInviteRespond,
		ActionExport,
		ActionBackupCreate,
		ActionBackupRestore,
		ActionEmailChange,
		ActionSwitchPatient,
	}

	targetKinds = []TargetKind{
		TargetKindMedication,
		TargetKindAllergy,
		TargetKindCondition,
		TargetKindEncounter,
		TargetKindProcedure,
		TargetKindTreatment,
		TargetKindSymptom,
		TargetKindVitals,
		TargetKindImmunization,
		TargetKindInjury,
		TargetKindInsurance,
		TargetKindEquipment,
		TargetKindEmergencyContact,
		TargetKindFamilyMember,
		TargetKindLabResult,
		TargetKindPatient,
		TargetKindUser,
		TargetKindShare,
		TargetKindInvitation,
		TargetKindAttachment,
		TargetKindExport,
		TargetKindBackup,
		TargetKindSystem,
		TargetKindPractitioner,
		TargetKindFacility,
		TargetKindTag,
		TargetKindSearch,
	}
)

// ActorKinds is what the migration writes into the select field, and what a
// later phase's migration test asserts the complete set against. It clones, as
// the two accessors below do, because a caller that sorted the result would
// otherwise reorder the vocabulary for the migration too.
func ActorKinds() []ActorKind { return slices.Clone(actorKinds) }

func Actions() []Action         { return slices.Clone(actions) }
func TargetKinds() []TargetKind { return slices.Clone(targetKinds) }

// Valid is false for the empty string on all three. Every one of them is a
// required column, so absence is Validate's business and is reported as
// required rather than as an unpublished value.
func (k ActorKind) Valid() bool  { return slices.Contains(actorKinds, k) }
func (a Action) Valid() bool     { return slices.Contains(actions, a) }
func (t TargetKind) Valid() bool { return slices.Contains(targetKinds, t) }
