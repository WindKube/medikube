package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/identity"
)

// The field codes this package raises. The shared ones — required, too_short,
// invalid_value — stay in internal/domain with every other entity's copy of
// them; these two name an operation's outcome rather than a value's shape,
// which is why they are not on identity.User.
const (
	// CodeIncorrect is a supplied password that is not the account's
	// (contracts/account.md).
	CodeIncorrect = "incorrect"

	// CodeMismatch is a confirmation phrase that is not the one asked for.
	CodeMismatch = "mismatch"
)

// The fields the account operations report a refusal on. They are the wire
// spellings contracts/account.md publishes, named here so that the service, the
// DTO and the form cannot each pick their own.
const (
	FieldCurrentPassword = "current_password"
	FieldConfirmation    = "confirmation"
	FieldPassword        = "password"
)

// passwordChangeRefusal is ONE message for two different failures, and that is
// the requirement rather than an economy (T197, FR-009).
//
// It is a single constant reported on both fields so that the wrong current
// password and an unacceptable new one are answered identically: two messages
// would let a caller learn which half it got right. The rules themselves are
// published by GET /api/v1/auth/config, so a person who wants to know what a
// new password must look like is told without having to fail at one.
const passwordChangeRefusal = "the current password is not right, or the new one does not meet the published rules"

var (
	// ErrRegistrationClosed is FR-002's refusal. It wraps domain.ErrForbidden
	// so that an edge which has not learned the specific code still answers 403
	// rather than 500 — the failure of a forgotten mapping should be a blunt
	// refusal, not an internal error with a stack trace behind it.
	//
	// Whether an operator opened self-registration is instance-wide
	// configuration, identical for every caller, so there is nothing here to
	// answer as a 404: a 404 is what this codebase answers for owner-scoped
	// data, where the existence of the thing is the secret (D15).
	ErrRegistrationClosed = fmt.Errorf("identity: self-registration is closed on this instance: %w", domain.ErrForbidden)

	// ErrInvalidToken is the ONE refusal for a link that has expired, has
	// already been used, or has been altered (FR-074, contracts/auth.md).
	//
	// It wraps no domain sentinel because none of them is a 400: the edge maps
	// it to web.ErrInvalidToken, and errors.Is against this value is how.
	// Telling the three cases apart would tell somebody which tokens once
	// existed.
	ErrInvalidToken = errors.New("identity: that link cannot be used")
)

// Service is the account use cases: registration, sign-in, sign-out, the
// profile, the password, deletion and recovery.
//
// Every method that concerns an existing account takes an access.Actor and
// reaches exactly one account — the actor's own. There is no id parameter
// anywhere in this package, which is FR-032 enforced by shape: there is no
// other account to name, so there is none to guess (contracts/account.md).
//
// It mints no token and hashes no password. The session a caller ends up
// holding comes from the edge, through PocketBase's own auth response, because
// that is what fires the hook that audits a sign-in on BOTH MediKube's route
// and PocketBase's native one (research D-13, D-14).
type Service struct {
	repository    Repository
	authenticator Authenticator
	mailer        Mailer
	auditor       Auditor
	clock         Clock

	registrationOpen bool
	supportedLocale  func(string) bool
}

// Config is what the service is wired with. A struct rather than six positional
// arguments, because five of them are interfaces and a transposed pair would
// compile.
//
// The zero value is closed. FR-002's default is closed and
// MEDIKUBE_AUTH_REGISTRATION_OPEN defaults to false, so a composition root that
// forgets to say gets the safe answer rather than an open instance.
type Config struct {
	Repository    Repository
	Authenticator Authenticator
	Mailer        Mailer
	Auditor       Auditor
	Clock         Clock

	// RegistrationOpen is the operator's instance-wide switch (FR-002). It is a
	// value and not a port because it is configuration read once at boot, and a
	// port would be a second place an operator could change it from.
	RegistrationOpen bool

	// SupportedLocale reports whether a locale UpdateProfile is asked to store
	// is one this instance ships a catalogue for (FR-001). It is wired with
	// i18n.IsSupported rather than imported directly: internal/service never
	// imports internal/i18n, so the membership question is asked through an
	// interface this package declares itself, the same reasoning as every
	// other port here.
	//
	// Nil accepts any locale identity.User.Validate's own format check
	// already lets through — a caller wiring this service for something that
	// never touches a locale should not have to know about a catalogue it
	// never asked about.
	SupportedLocale func(string) bool
}

// New refuses an incomplete service rather than returning one.
//
// A nil authenticator is a service that would panic on the first sign-in, after
// the process has been serving traffic for however long it took somebody to
// reach it. The composition root gets the error instead, at boot.
func New(cfg Config) (*Service, error) {
	var missing []string

	if cfg.Repository == nil {
		missing = append(missing, "repository")
	}

	if cfg.Authenticator == nil {
		missing = append(missing, "authenticator")
	}

	if cfg.Mailer == nil {
		missing = append(missing, "mailer")
	}

	if cfg.Auditor == nil {
		missing = append(missing, "auditor")
	}

	if cfg.Clock == nil {
		missing = append(missing, "clock")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("identity: the service is wired with no %s", joinWords(missing))
	}

	return &Service{
		repository:       cfg.Repository,
		authenticator:    cfg.Authenticator,
		mailer:           cfg.Mailer,
		auditor:          cfg.Auditor,
		clock:            cfg.Clock,
		registrationOpen: cfg.RegistrationOpen,
		supportedLocale:  cfg.SupportedLocale,
	}, nil
}

// RegistrationOpen is what GET /api/v1/auth/config publishes (contracts/auth.md).
// It says nothing about anybody: whether the door is open is not a fact about
// who is inside.
func (s *Service) RegistrationOpen() bool { return s.registrationOpen }

// Register creates an account and returns it (FR-001).
//
// The refusal when registration is closed comes FIRST, before the submission is
// so much as looked at: a closed instance that answered `422 invalid email` and
// `403 registration_closed` on different bodies would be running its validator
// for anonymous callers, and would confirm which addresses parse.
func (s *Service) Register(ctx context.Context, actor access.Actor, registration Registration) (identity.User, error) {
	if !s.registrationOpen {
		return identity.User{}, ErrRegistrationClosed
	}

	draft := identity.User{
		Email:      strings.TrimSpace(registration.Email),
		Name:       strings.TrimSpace(registration.Name),
		Role:       identity.DefaultRole,
		UnitSystem: identity.DefaultUnitSystem,
		Locale:     s.registrationLocale(registration.Locale),
		DateFormat: identity.DefaultDateFormat,
		Theme:      identity.DefaultTheme,
	}

	// Both checks run and both are reported, which is FR-027: a person fixing
	// one problem per round trip because the server only mentioned one is four
	// wasted attempts to say what it knew at the first.
	if err := merge(
		draft.Validate(),
		identity.ValidatePassword(registration.Password, draft.Email, draft.Name),
	); err != nil {
		return identity.User{}, err
	}

	stored, err := s.repository.Create(ctx, draft, registration.Password)
	if err != nil {
		return identity.User{}, err
	}

	if err := s.record(ctx, actor, event{
		action:  audit.ActionCreate,
		actorID: stored.ID,
		target:  stored.ID,
	}); err != nil {
		return identity.User{}, err
	}

	return stored, nil
}

// Me is the account behind the session (contracts/account.md, getMe).
func (s *Service) Me(ctx context.Context, actor access.Actor) (identity.User, error) {
	return s.account(ctx, actor)
}

// UpdateProfile changes the display name and the four preferences, and nothing
// else (FR-011).
//
// It reads the stored account and applies the patch over it, so every member a
// request may not set keeps the value the store holds. That the patch has no
// member for the role, the address, the confirmed state or the disabled instant
// is the enforcement (FR-012); this is where it becomes a stored fact.
func (s *Service) UpdateProfile(ctx context.Context, actor access.Actor, profile Profile) (identity.User, error) {
	current, err := s.account(ctx, actor)
	if err != nil {
		return identity.User{}, err
	}

	changed := profile.applyTo(current)

	formatErr := changed.Validate()

	// A locale the format check already rejected ("english" is not a
	// two-letter code) is not asked about membership too — one field, one
	// refusal, or a malformed locale would be reported twice under the same
	// field (T197's reasoning, applied here).
	localeErr := s.validateLocale(changed.Locale)
	if fieldInvalid(formatErr, "locale") {
		localeErr = nil
	}

	if invalid := merge(formatErr, localeErr); invalid != nil {
		return identity.User{}, invalid
	}

	updated, err := s.repository.Update(ctx, changed)
	if err != nil {
		return identity.User{}, err
	}

	if err := s.record(ctx, actor, event{action: audit.ActionUpdate, target: updated.ID}); err != nil {
		return identity.User{}, err
	}

	return updated, nil
}

// ChangePassword replaces the credential, and only on proof that it is still
// the same person at the keyboard (FR-009).
//
// Both checks are made before either is reported, and the refusal is one
// message on both fields (T197). The order matters for a second reason: the
// bcrypt comparison happens on every attempt, valid new password or not, so the
// refusal cannot be timed to say which half failed.
//
// The successful change rotates the record's token key, which ends every
// session issued before it (FR-010). That rotation is the Authenticator's, in
// the same write that stores the hash — this method's contribution is that
// there is no other path to a new password.
func (s *Service) ChangePassword(ctx context.Context, actor access.Actor, current, next string) error {
	user, err := s.account(ctx, actor)
	if err != nil {
		return err
	}

	supplied := s.authenticator.Verify(ctx, user.ID, current)
	if supplied != nil && !errors.Is(supplied, domain.ErrUnauthenticated) {
		return fmt.Errorf("identity: the current password could not be checked: %w", supplied)
	}

	chosen := identity.ValidatePasswordField(identity.FieldNewPassword, next, user.Email, user.Name)

	if supplied != nil || chosen != nil {
		return refusedPasswordChange()
	}

	if err := s.authenticator.SetPassword(ctx, user.ID, next); err != nil {
		return fmt.Errorf("identity: the new password was not stored: %w", err)
	}

	return s.record(ctx, actor, event{action: audit.ActionPasswordChange, target: user.ID})
}

// DeleteAccount is the one irreversible operation in this phase (FR-013,
// FR-014).
//
// The audit row is written BEFORE the delete, deliberately. `audit_events.actor`
// does not cascade, so the row outlives the account with its actor unset and
// `actor_kind` as the surviving evidence that a person did it (research D-22).
// Written afterwards it could not be written at all: the relation would point
// at a record that no longer exists.
func (s *Service) DeleteAccount(ctx context.Context, actor access.Actor, password, confirmation string) error {
	user, err := s.account(ctx, actor)
	if err != nil {
		return err
	}

	var invalid domain.ValidationError

	// Compared exactly: not trimmed, not folded. An irreversible act asks for a
	// deliberate one in return (FR-013).
	if confirmation != identity.DeleteConfirmationPhrase {
		invalid.Addf(FieldConfirmation, CodeMismatch,
			"type %s exactly as it is shown to delete this account", identity.DeleteConfirmationPhrase)
	}

	switch supplied := s.authenticator.Verify(ctx, user.ID, password); {
	case supplied == nil:
	case errors.Is(supplied, domain.ErrUnauthenticated):
		invalid.Add(FieldPassword, CodeIncorrect, "that is not this account's password")
	default:
		return fmt.Errorf("identity: the password could not be checked: %w", supplied)
	}

	if err := invalid.OrNil(); err != nil {
		return err
	}

	if err := s.record(ctx, actor, event{action: audit.ActionAccountDelete, target: user.ID}); err != nil {
		return err
	}

	return s.repository.Delete(ctx, user.ID)
}

// registrationLocale is FR-004's fallback: an empty or unrecognised locale
// falls back to identity.DefaultLocale rather than failing the sign-up — the
// account still gets created, just in English, so a caller's browser
// (something outside anybody's control) never blocks the one thing this
// request is for.
func (s *Service) registrationLocale(locale string) string {
	if locale == "" || s.supportedLocale == nil || !s.supportedLocale(locale) {
		return identity.DefaultLocale
	}

	return locale
}

// validateLocale reports whether locale is one this instance ships a
// catalogue for. It is a separate check from identity.User.Validate's own
// format rule: "xx" is a well-formed two-letter code and passes that check,
// but is not a language anything renders in.
func (s *Service) validateLocale(locale string) error {
	if s.supportedLocale == nil || s.supportedLocale(locale) {
		return nil
	}

	var invalid domain.ValidationError
	invalid.Add("locale", domain.CodeInvalidValue, "that is not a language this instance offers")

	return invalid.OrNil()
}

// fieldInvalid reports whether err already carries a refusal for field, so a
// second check on the same field does not report it twice.
func fieldInvalid(err error, field string) bool {
	var invalid *domain.ValidationError
	if !errors.As(err, &invalid) {
		return false
	}

	for _, f := range invalid.Fields {
		if f.Field == field {
			return true
		}
	}

	return false
}

// account is the checkpoint every account operation passes, and the one place
// the actor is turned into a stored account.
//
// There is no record id to authorize against — the only account any of these
// operations can reach is the actor's own — so what this refuses is a caller
// that has no MediKube account at all, and an account an operator has taken out
// of service.
func (s *Service) account(ctx context.Context, actor access.Actor) (identity.User, error) {
	if err := authorize(actor); err != nil {
		return identity.User{}, err
	}

	user, err := s.repository.Get(ctx, actor.UserID)
	if err != nil {
		return identity.User{}, err
	}

	// data-model §1: a non-zero instant refuses sign-in. It has to refuse the
	// session too, or an account disabled while somebody was signed in stays
	// fully usable until their token expires — PocketBase's token validation
	// never looks at this column (FACT: core/record_query.go's
	// FindAuthRecordByToken evaluates no collection rule).
	if user.IsDisabled() {
		return identity.User{}, fmt.Errorf("identity: the account is not in service: %w", domain.ErrUnauthenticated)
	}

	return user, nil
}

// authorize refuses a caller with no MediKube account.
//
// A PocketBase superuser is refused along with an anonymous one, and that is
// deliberate: it is the break-glass credential and not a MediKube role
// (data-model §1, FR-040), it has no row in `users`, and MediKube's own routes
// are not a second, unaudited way into somebody's account. internal/service/access
// refuses it at the record checkpoint for the same reason.
func authorize(actor access.Actor) error {
	switch {
	case !actor.Authenticated():
		return fmt.Errorf("identity: the request carries no account: %w", domain.ErrUnauthenticated)
	case actor.IsSuperuser:
		return fmt.Errorf("identity: a superuser session holds no MediKube account: %w", domain.ErrUnauthenticated)
	default:
		return nil
	}
}

// event is one audit row's variable half. The constants — the actor kind, the
// target kind, the clock and the correlation id — are filled in by record, so
// that no call site can write a row about a medication or forget the request it
// belongs to.
type event struct {
	action audit.Action

	// actorID overrides the actor's own id, for the one row whose subject is an
	// account that did not exist when the request started.
	actorID string

	target string
}

func (s *Service) record(ctx context.Context, actor access.Actor, e event) error {
	actorID := actor.UserID
	if e.actorID != "" {
		actorID = e.actorID
	}

	row := audit.Event{
		OccurredAt: s.clock.Now().UTC(),
		ActorID:    actorID,
		ActorKind:  audit.ActorKindUser,
		Action:     e.action,
		TargetKind: audit.TargetKindUser,
		TargetID:   e.target,
		RequestID:  actor.RequestID,
	}

	if actor.IsSuperuser {
		row.ActorKind = audit.ActorKindSuperuser
	}

	if err := s.auditor.Record(ctx, row); err != nil {
		return fmt.Errorf("identity: the %s was not recorded: %w", e.action, err)
	}

	return nil
}

// applyTo returns the account as it would be with the patch applied, over a
// copy, so a refused change leaves the caller's copy as it read it.
func (p Profile) applyTo(user identity.User) identity.User {
	assign(&user.Name, p.Name)
	assign(&user.UnitSystem, p.UnitSystem)
	assign(&user.Locale, p.Locale)
	assign(&user.DateFormat, p.DateFormat)
	assign(&user.Theme, p.Theme)

	return user
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}

// refusedPasswordChange is the ONE refusal, built in one place so that the two
// branches cannot answer differently under a later edit (T197).
func refusedPasswordChange() error {
	var invalid domain.ValidationError

	invalid.Add(FieldCurrentPassword, CodeIncorrect, passwordChangeRefusal)
	invalid.Add(identity.FieldNewPassword, CodeIncorrect, passwordChangeRefusal)

	return invalid.OrNil()
}

// merge reports every offending field of several checks at once (FR-027). A
// failure that is not a validation error is not something to collect: it is
// returned as it is, because it is a condition rather than a person's mistake.
func merge(errs ...error) error {
	var invalid domain.ValidationError

	for _, err := range errs {
		if err == nil {
			continue
		}

		var fields *domain.ValidationError
		if !errors.As(err, &fields) {
			return err
		}

		invalid.Fields = append(invalid.Fields, fields.Fields...)
	}

	return invalid.OrNil()
}

func joinWords(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinWords(words[1:])
	}
}
