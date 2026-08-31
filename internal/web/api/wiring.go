package api

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/httproute"
	"medikube/internal/platform/pb"
	serviceaudit "medikube/internal/service/audit"
	serviceidentity "medikube/internal/service/identity"
	storeaudit "medikube/internal/store/audit"
	storeidentity "medikube/internal/store/identity"
	"medikube/internal/web"
)

// AccountsConfig is the operator's half of the identity stack: the two settings
// that decide who may create an account and how long a session lasts, and the
// public URL that decides whether the cookie is Secure.
type AccountsConfig struct {
	RegistrationOpen bool
	SessionTTL       time.Duration
	PublicURL        string

	// Resolve is the record family, for the account's own record counts.
	Resolve Resolve
}

// Accounts is one assembled identity stack.
//
// Authenticator is exposed because the anti-enumeration mechanism is a
// PRODUCTION property with a counting seam on it: an address with no account
// still pays for a bcrypt comparison against a fixed dummy hash, and T202
// asserts that by counting the comparisons rather than by timing them
// (Constitution VIII forbids a clock in a gate). A mechanism nothing can
// observe is a mechanism nothing notices the deletion of.
type Accounts struct {
	Deps          Deps
	Service       *serviceidentity.Service
	Authenticator *storeidentity.Authenticator
}

// NewAccounts assembles the identity stack for one instance.
//
// It lives here rather than in each composition root because there are two of
// them — cmd/medikube and internal/web/apitest — and an application the tests
// assemble differently from the one that ships is an application the tests do
// not describe. The record family is duplicated between the two for historical
// reasons; this one is not.
func NewAccounts(app core.App, cfg AccountsConfig) (*Accounts, error) {
	if app == nil {
		return nil, errors.New("api: the identity stack is wired with no application")
	}

	repository, err := storeidentity.NewRepository(app)
	if err != nil {
		return nil, err
	}

	authenticator, err := storeidentity.NewAuthenticator(app)
	if err != nil {
		return nil, err
	}

	mailer, err := pb.NewMailer(app)
	if err != nil {
		return nil, err
	}

	trail, err := storeaudit.New(app)
	if err != nil {
		return nil, err
	}

	auditor, err := serviceaudit.New(trail)
	if err != nil {
		return nil, err
	}

	service, err := serviceidentity.New(serviceidentity.Config{
		Repository:       repository,
		Authenticator:    authenticator,
		Mailer:           mailer,
		Auditor:          auditor,
		Clock:            serviceidentity.SystemClock{},
		RegistrationOpen: cfg.RegistrationOpen,
	})
	if err != nil {
		return nil, err
	}

	sessions, err := NewSessionWriter(app, web.NewSessionCookie(cfg.SessionTTL, cfg.PublicURL))
	if err != nil {
		return nil, err
	}

	counts, err := NewCounter(cfg.Resolve)
	if err != nil {
		return nil, err
	}

	return &Accounts{
		Deps: Deps{
			Accounts: service,
			Sessions: sessions,
			Counts:   counts,
			Mail:     pb.MailConfigured(app),
		},
		Service:       service,
		Authenticator: authenticator,
	}, nil
}

// AccountOperations is every operation id the auth and account halves serve.
//
// It exists because the composition root has to know what is wired BEFORE it
// can build it: the identity stack needs a running application and the stub
// inventory does not. Handlers asserts that this list and the table it builds
// are the same set, so the two cannot drift into a route that answers 501
// while a handler for it sits unreachable.
func AccountOperations() []string {
	return []string{
		OpGetAuthConfig, OpRegister, OpLogin, OpRefreshSession, OpLogout,
		OpRequestPasswordReset, OpConfirmPasswordReset,
		OpRequestEmailVerification, OpConfirmEmailVerification,
		OpGetMe, OpUpdateMe, OpChangePassword, OpDeleteMe,
	}
}

// Handlers is the thirteen operations of contracts/auth.md and
// contracts/account.md.
func (a *Accounts) Handlers() (httproute.Handlers, error) {
	auth, err := AuthHandlers(a.Deps)
	if err != nil {
		return nil, err
	}

	account, err := AccountHandlers(a.Deps)
	if err != nil {
		return nil, err
	}

	table := make(httproute.Handlers, len(auth)+len(account))

	for opID, handler := range auth {
		table[opID] = handler
	}

	for opID, handler := range account {
		if _, taken := table[opID]; taken {
			return nil, fmt.Errorf("api: %s is wired by both halves of the account surface", opID)
		}

		table[opID] = handler
	}

	for _, opID := range AccountOperations() {
		if _, wired := table[opID]; !wired {
			return nil, fmt.Errorf("api: %s is published as an account operation and nothing serves it", opID)
		}
	}

	if len(table) != len(AccountOperations()) {
		return nil, fmt.Errorf(
			"api: the account surface serves %d operations and publishes %d; AccountOperations has drifted",
			len(table), len(AccountOperations()))
	}

	return table, nil
}
