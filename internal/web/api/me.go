package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/httproute"
	"medikube/internal/records"
	"medikube/internal/service/patient"
	"medikube/internal/web"
)

// The operation ids of contracts/account.md's four operations.
const (
	OpGetMe          = "getMe"
	OpUpdateMe       = "updateMe"
	OpChangePassword = "changePassword"
	OpDeleteMe       = "deleteMe"
)

// ErrNoCount is a counter that answered without the count it was asked for. It
// is an internal failure: the deletion confirmation states how many records
// will be destroyed, and a confirmation that guessed would be worse than none.
var ErrNoCount = errors.New("api: the record count was asked for and not answered")

type accountHandlers struct {
	deps Deps
}

// AccountHandlers is contracts/account.md's four operations.
//
// There is no id parameter anywhere among them, and that is FR-032 enforced by
// shape: the only account any of these can reach is the one the session names,
// so there is no other to guess and no `DELETE /api/v1/users/{id}` to guard.
func AccountHandlers(deps Deps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	h := &accountHandlers{deps: deps}

	return httproute.Handlers{
		OpGetMe:          web.WithActor(h.get),
		OpUpdateMe:       web.WithActor(h.update),
		OpChangePassword: web.WithActor(h.changePassword),
		OpDeleteMe:       web.WithActor(h.remove),
	}, nil
}

// PatientLister lists the ids of every patient one account owns.
//
// Every clinical kind the record family now serves is patient-anchored
// (research D-13): "the account's records" is the union over its patients, so
// NewCounter needs this to sum a kind's count rather than asking the family
// for a total it can no longer answer with no patient named.
type PatientLister func(ctx context.Context, ownerID string) ([]string, error)

// NewCounter resolves the account's own record counts through the record
// family, one patient at a time.
//
// Through the family and not through a query of its own, deliberately: each
// patient's count still passes the SAME authorization checkpoint every record
// read passes, with the actor as its only input. A second COUNT(*) written
// here would be a second place the patient scope could be got wrong, and a
// count is exactly the kind of read nobody thinks of as a disclosure until it
// is one.
func NewCounter(resolve Resolve, patientsOf PatientLister) (Counter, error) {
	if resolve == nil {
		return nil, ErrNoRecords
	}

	if patientsOf == nil {
		return nil, fmt.Errorf("api: the account's record counter is wired with no patient lister")
	}

	return func(ctx context.Context, actor access.Actor) (MeCounts, error) {
		handler, err := resolve()
		if err != nil {
			return nil, err
		}

		patientIDs, err := patientsOf(ctx, actor.UserID)
		if err != nil {
			return nil, err
		}

		// Every kind this build serves, so the deletion confirmation states
		// what will be destroyed rather than what somebody remembered to add a
		// member for. The segments come from the handler and not from
		// kind.Kinds(): a kind the registry does not carry has no repository to
		// count through, and asking for one is an error rather than a zero.
		counts := make(MeCounts, len(handler.Segments()))

		for _, segment := range handler.Segments() {
			var total int

			for _, patientID := range patientIDs {
				page, err := handler.ListOfKind(ctx, actor, segment, records.Query{
					PatientID: patientID,
					// One row, because the answer wanted is the total beside
					// it. The page itself is discarded.
					Limit: 1,
					Count: true,
				})
				if err != nil {
					return nil, web.OwnerScoped(err)
				}

				if page.Total == nil {
					return nil, fmt.Errorf("%w: %s", ErrNoCount, segment)
				}

				total += *page.Total
			}

			counts[segment] = total
		}

		return counts, nil
	}, nil
}

// get is the signed-in account (contracts/account.md).
//
// `private, no-store` and no ETag: the body is the person's own profile, and
// there is no concurrency question for a validator to answer.
func (h *accountHandlers) get(e *core.RequestEvent, actor access.Actor) error {
	me, err := h.me(e, actor)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", accountCacheControl)

	return web.WriteJSON(e, http.StatusOK, me)
}

// update changes the display name and the four preferences, and nothing else
// (FR-011).
//
// What it CANNOT change is not enforced here. MePatch has no member for the
// address, the role, the confirmed state or the disabled instant, and unknown
// members are refused by the decoder, so a body carrying any of them is 422
// before this function runs (FR-012). me_privilege_test.go asserts that against
// every column the account collection has, not only against the ones anybody
// thought to name.
func (h *accountHandlers) update(e *core.RequestEvent, actor access.Actor) error {
	var patch MePatch
	if err := web.Decode(e, &patch); err != nil {
		return err
	}

	user, err := h.deps.Accounts.UpdateProfile(e.Request.Context(), actor, patch.Profile())
	if err != nil {
		return refused(err)
	}

	counts, err := h.deps.Counts(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	activePatient, ownedCount, err := h.activePatient(e.Request.Context(), actor)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", accountCacheControl)

	return web.WriteJSON(e, http.StatusOK, NewMe(user, counts, activePatient, ownedCount))
}

// changePassword replaces the credential and re-issues the caller's own session
// (FR-009, FR-010).
//
// The ORDER is the whole of it and it is easy to get backwards: the service
// validates the current password, sets the new one and saves, which rotates the
// record's token key and invalidates every outstanding token for the account —
// including the one this request arrived with. The fresh cookie is then minted
// from the SAVED record, so the person who made the change stays signed in
// where they made it and every other session stops working.
//
// It answers 204 with a Set-Cookie and no body. The session writer is still
// what mints it: MediKube mints no token anywhere.
func (h *accountHandlers) changePassword(e *core.RequestEvent, actor access.Actor) error {
	var body ChangePasswordRequest
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	if err := h.deps.Accounts.ChangePassword(
		e.Request.Context(), actor, body.CurrentPassword, body.NewPassword); err != nil {
		return refused(err)
	}

	// The empty authMethod, for the same reason refresh uses it: this is not a
	// sign-in and must not write a `login` row. The row it writes is
	// `password_change`, from the service.
	return h.deps.Sessions.Issue(e, actor.UserID, "", reissued())
}

// remove is the one irreversible operation in this phase (FR-013, FR-014).
//
// The account and every medication under it go in one transaction — the
// cascade on `medications.owner` is PocketBase's, and it is asserted directly
// against stored data rather than trusted (me_delete_integration_test.go). The
// `account_delete` audit row is written BEFORE the delete and does not cascade,
// so the evidence outlives the account.
func (h *accountHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	var body DeleteAccountRequest
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	if err := h.deps.Accounts.DeleteAccount(
		e.Request.Context(), actor, body.Password, body.Confirmation); err != nil {
		return refused(err)
	}

	h.deps.Sessions.Clear(e)
	e.Response.Header().Set("Cache-Control", accountCacheControl)

	return e.NoContent(http.StatusNoContent)
}

func (h *accountHandlers) me(e *core.RequestEvent, actor access.Actor) (Me, error) {
	user, err := h.deps.Accounts.Me(e.Request.Context(), actor)
	if err != nil {
		return Me{}, refused(err)
	}

	counts, err := h.deps.Counts(e.Request.Context(), actor)
	if err != nil {
		return Me{}, err
	}

	activePatient, ownedCount, err := h.activePatient(e.Request.Context(), actor)
	if err != nil {
		return Me{}, err
	}

	return NewMe(user, counts, activePatient, ownedCount), nil
}

// activePatient answers contracts/active-patient.md's amendment to getMe: the
// person in view, resolved (and auto-selected, FR-018) through the same
// checkpoint every read of a patient passes, and how many the account owns.
func (h *accountHandlers) activePatient(ctx context.Context, actor access.Actor) (*PatientSummary, int, error) {
	return resolveActivePatient(ctx, actor, h.deps.Patients)
}

// resolveActivePatient is shared by getMe and by the account body a
// registration or a sign-in answers with (auth.go's render): both amend the
// same wire shape, and a resolver written twice is two places one of them
// could disagree with FR-018's auto-selection.
func resolveActivePatient(ctx context.Context, actor access.Actor, resolve PatientResolve) (*PatientSummary, int, error) {
	svc, err := resolve()
	if err != nil {
		return nil, 0, err
	}

	resolved, err := svc.ResolveActivePatient(ctx, actor)
	if err != nil {
		return nil, 0, err
	}

	page, err := svc.List(ctx, actor, patient.Query{Limit: 1, Count: true})
	if err != nil {
		return nil, 0, err
	}

	var ownedCount int
	if page.Total != nil {
		ownedCount = *page.Total
	}

	if resolved == nil {
		return nil, ownedCount, nil
	}

	summary := NewPatientSummary(*resolved, nil)

	return &summary, ownedCount, nil
}
