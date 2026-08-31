package pb

import (
	"context"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"

	service "medikube/internal/service/identity"
)

// Mailer is the outgoing-message adapter behind the identity service's Mailer
// port (T223j).
//
// It renders nothing and mints nothing. PocketBase builds the message from the
// collection's own template, mints the token the link carries and signs it with
// the record's token key — which is why a recovery link stops working the
// moment the password it set is saved, with no state stored anywhere.
//
// Both methods take an account id and nothing else. An address parameter would
// let a signed-in caller aim this instance's mailer at a stranger, and the
// address is on the record already.
type Mailer struct {
	app core.App
}

var _ service.Mailer = (*Mailer)(nil)

func NewMailer(app core.App) (*Mailer, error) {
	if app == nil {
		return nil, errors.New("pb: the mailer is wired with no application, so nothing it accepted would be sent")
	}

	return &Mailer{app: app}, nil
}

// SendPasswordReset sends the recovery message for one account (FR-073).
//
// SYNCHRONOUS, unlike PocketBase's own route, which sends through
// routine.FireAndForget and logs a failure where nobody sees it
// (apis/record_auth_password_reset_request.go). The caller here needs the
// error: it is what the operator is told about a mailer that is not working,
// and FR-076 forbids reporting a message as sent when it was not.
//
// It is NOT what the caller of the HTTP operation is told. The edge answers 202
// either way and logs this, because an error only a registered address could
// provoke is an account-existence oracle (FR-073).
func (m *Mailer) SendPasswordReset(ctx context.Context, userID string) error {
	record, err := m.account(ctx, userID)
	if err != nil {
		return err
	}

	if err := mails.SendRecordPasswordReset(m.app, record); err != nil {
		return fmt.Errorf("pb: sending the recovery message: %w", err)
	}

	return nil
}

// SendVerification sends the address-confirmation message (FR-075).
func (m *Mailer) SendVerification(ctx context.Context, userID string) error {
	record, err := m.account(ctx, userID)
	if err != nil {
		return err
	}

	if err := mails.SendRecordVerification(m.app, record); err != nil {
		return fmt.Errorf("pb: sending the confirmation message: %w", err)
	}

	return nil
}

func (m *Mailer) account(ctx context.Context, userID string) (*core.Record, error) {
	record, err := m.app.FindRecordById(usersCollection, userID)
	if err != nil {
		return nil, fmt.Errorf("pb: reading the account a message was to be sent to: %w", err)
	}

	// The context is honoured by declaring it rather than by threading it into
	// a call that has nowhere to take one: neither mails.SendRecord* nor
	// FindRecordById accepts one in v0.40.1. Checking it here is what keeps a
	// cancelled request from starting an SMTP conversation nobody is waiting
	// for.
	return record, ctx.Err()
}

// MailConfigured reports whether this instance can send outgoing mail at all
// (FR-076).
//
// It reads PocketBase's persisted settings rather than MEDIKUBE_*, which is the
// carve-out the constitution's Technology Constraints make for exactly this:
// SMTP is part of the platform. When it is off, app.NewMailClient() returns a
// shell-out to a local `sendmail` binary the distroless image does not contain,
// so the send fails rather than silently disappearing — and the handlers refuse
// beforehand instead of accepting a request they cannot honour.
func MailConfigured(app core.App) func() bool {
	return func() bool {
		if app == nil {
			return false
		}

		return app.Settings().SMTP.Enabled
	}
}
