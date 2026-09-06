package page

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	"medikube/internal/service/identity"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/settings"
	"medikube/internal/web/views/shell"
)

// The operation ids of contracts/pages.md's P1, P2, P6, P7, P8 and P9, spelled
// as internal/httproute declares them.
const (
	OpLoginPage          = "loginPage"
	OpRegisterPage       = "registerPage"
	OpSettingsPage       = "settingsPage"
	OpForgotPasswordPage = "forgotPasswordPage"
	OpResetPasswordPage  = "resetPasswordPage"
	OpVerifyEmailPage    = "verifyEmailPage"
)

// The other five's titles are resolved at render time via i18n.T against the
// same ids their signed-out nav labels and landmarks use (action.sign_in,
// auth.create_account, auth.reset_password, auth.choose_new_password,
// auth.confirm_your_address); P9's title is not its landmark: the gate
// resolves region[name="Email confirmation"] and the tab says "Confirm your
// address", and contracts/pages.md fixes both.

// PathToken is the {token} segment of P8 and P9, spelled as the route table
// declares it. A handler that read a differently spelled parameter would see an
// empty token and answer the dead-link state to everybody, which looks exactly
// like a page that works.
const PathToken = "token"

// ErrNoAccountPages is a build whose account pages were wired without the
// service that answers them.
var ErrNoAccountPages = errors.New("page: the account pages were wired without an identity service")

// AccountDeps is what the six account pages need.
//
// They are the one group of pages that cannot be built out of the kind registry
// alone: the settings page renders the signed-in account, the sign-up page
// renders the operator's switch, and the two token pages resolve the link they
// were opened with — so all of them need what the composition root assembles
// from a running application.
type AccountDeps struct {
	// Accounts is the identity service, for the account itself and for whether
	// the operator has opened self-registration. It is the concrete type for
	// the same reason internal/web/api holds it as one: an interface over it
	// would have thirteen methods and one implementation.
	Accounts *identity.Service

	// Counts is what the danger zone states will be destroyed. It is the same
	// counter the API answers getMe with, so the page and the JSON cannot
	// disagree about what an account holds.
	Counts api.Counter

	// Mail is whether this instance can send anything (FR-076). It is the same
	// question the auth handlers refuse on, asked here so that the recovery
	// page explains itself instead of collecting an address for a message
	// nobody could send.
	Mail api.MailConfigured

	// Links is how the two token pages know which state to render.
	Links Links
}

// Links resolves a recovery or confirmation link to the account it was minted
// for, WITHOUT spending it.
//
// Declared here, by the consumer, and satisfied by the same authenticator the
// identity service redeems through — so the page's answer to "does this link
// still work" is the answer the API will give when the form is submitted, and
// not a second opinion formed from the shape of the token.
//
// The question has to be asked at GET time. FR-074 requires a link that has
// expired, been used or been altered to be explained INSIDE the landmark with
// the offer to request another, and contracts/pages.md points both smoke URLs
// at a deliberately dead token; a page that only learned the answer from the
// submission would render the form, take a new password and refuse it
// afterwards.
type Links interface {
	Redeem(ctx context.Context, purpose identity.TokenPurpose, token string) (domainidentity.User, error)
}

// AccountPageOperations is every page id this group serves.
//
// It exists for the composition root's benefit: the stub inventory has to know
// what is wired before the identity stack can be built, exactly as
// api.AccountOperations does for the API half.
func AccountPageOperations() []string {
	return []string{
		OpLoginPage, OpRegisterPage, OpSettingsPage,
		OpForgotPasswordPage, OpResetPasswordPage, OpVerifyEmailPage,
	}
}

// AccountPages is the six pages of the account surface.
//
// None of them is registered conditionally, and none of them answers anything
// but 200 on its own smoke URL. /register in particular is
// registered whether or not the operator has opened self-registration, and
// renders an explanation when they have not (FR-002, defect D15): a route that
// disappears under a configuration is a route the inventory gate cannot check,
// and a page that 404s under one is a page the browser gate would have to be
// told about.
func AccountPages(deps AccountDeps) (httproute.Handlers, error) {
	if deps.Accounts == nil {
		return nil, ErrNoAccountPages
	}

	if deps.Counts == nil {
		return nil, fmt.Errorf("%w: and with no record counter, so the deletion confirmation could state nothing", ErrNoAccountPages)
	}

	if deps.Mail == nil {
		return nil, fmt.Errorf("%w: and with no way to tell whether it can send mail, so the recovery page would offer a message nobody could send", ErrNoAccountPages)
	}

	if deps.Links == nil {
		return nil, fmt.Errorf("%w: and with nothing to resolve a recovery link, so both token pages would refuse every link", ErrNoAccountPages)
	}

	links, err := newAccountLinks()
	if err != nil {
		return nil, err
	}

	pages := &accountPages{deps: deps, links: links}

	table := httproute.Handlers{
		OpLoginPage:          web.WithActor(pages.login),
		OpRegisterPage:       web.WithActor(pages.register),
		OpSettingsPage:       web.WithActor(pages.settings),
		OpForgotPasswordPage: web.WithActor(pages.forgotPassword),
		OpResetPasswordPage:  web.WithActor(pages.resetPassword),
		OpVerifyEmailPage:    web.WithActor(pages.verifyEmail),
	}

	for _, opID := range AccountPageOperations() {
		if _, wired := table[opID]; !wired {
			return nil, fmt.Errorf("page: %s is published as an account page and nothing serves it", opID)
		}
	}

	return table, nil
}

type accountPages struct {
	deps  AccountDeps
	links accountLinks
}

// accountLinks holds every address the six pages need, each recovered from
// the route table rather than composed here — so a form posts to the route the
// router serves, by construction.
type accountLinks struct {
	loginPage    string
	registerPage string
	forgotPage   string
	settingsPage string
	medications  medicationLinks

	login        string
	register     string
	logout       string
	me           string
	password     string
	verify       string
	recover      string
	setPassword  string
	confirmEmail string
}

func newAccountLinks() (accountLinks, error) {
	medications, err := newMedicationLinks()
	if err != nil {
		return accountLinks{}, err
	}

	paths, err := routePaths(map[string]string{
		OpLoginPage:                    "",
		OpRegisterPage:                 "",
		OpSettingsPage:                 "",
		OpForgotPasswordPage:           "",
		api.OpLogin:                    "",
		api.OpRegister:                 "",
		api.OpLogout:                   "",
		api.OpGetMe:                    "",
		api.OpChangePassword:           "",
		api.OpRequestEmailVerification: "",
		api.OpRequestPasswordReset:     "",
		api.OpConfirmPasswordReset:     "",
		api.OpConfirmEmailVerification: "",
	})
	if err != nil {
		return accountLinks{}, err
	}

	return accountLinks{
		loginPage:    paths[OpLoginPage],
		registerPage: paths[OpRegisterPage],
		forgotPage:   paths[OpForgotPasswordPage],
		settingsPage: paths[OpSettingsPage],
		medications:  medications,

		login:        paths[api.OpLogin],
		register:     paths[api.OpRegister],
		logout:       paths[api.OpLogout],
		me:           paths[api.OpGetMe],
		password:     paths[api.OpChangePassword],
		verify:       paths[api.OpRequestEmailVerification],
		recover:      paths[api.OpRequestPasswordReset],
		setPassword:  paths[api.OpConfirmPasswordReset],
		confirmEmail: paths[api.OpConfirmEmailVerification],
	}, nil
}

// The Datastar actions the controls run. They are composed here and never in a
// template, for the same reason the record pages compose theirs: a view that
// built a path would be a second place a route is spelled.
//
// A submit names the fields it sends. Datastar's default body is every signal
// on the page, and the settings page carries three forms whose fields the
// other two forms' strict DTOs refuse as unknown members.
func (l accountLinks) post(path string) string { return "@post(" + quote(path) + ")" }

func (l accountLinks) patch(path string, fields ...string) string {
	return "@patch(" + quote(path) + payload(fields) + ")"
}

func (l accountLinks) put(path string, fields ...string) string {
	return "@put(" + quote(path) + payload(fields) + ")"
}

func (l accountLinks) remove(path string, fields ...string) string {
	return "@delete(" + quote(path) + payload(fields) + ")"
}

func payload(fields []string) string {
	if len(fields) == 0 {
		return ""
	}

	pairs := make([]string, len(fields))
	for i, field := range fields {
		pairs[i] = field + ": $" + field
	}

	return ", {payload: {" + strings.Join(pairs, ", ") + "}}"
}

// signedOutNav is contracts/pages.md's navigation for a page reached with no
// session: the landmark is on every page in the application and what changes is
// its CONTENTS, because phase 005's public invitation page is opened by
// somebody who needs exactly these two links.
func (l accountLinks) signedOutNav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: i18n.T(ctx, "action.sign_in"), Href: l.loginPage, Current: current == l.loginPage},
		{Label: i18n.T(ctx, "auth.create_account"), Href: l.registerPage, Current: current == l.registerPage},
	}
}

// signedInNav is the record kinds plus the account's own page. Settings is
// already one of medications.nav's entries — FR-050 requires it on every
// signed-in page, not only this one — so this is that list and nothing more.
func (l accountLinks) signedInNav(ctx context.Context, current string) []shell.NavLink {
	return l.medications.nav(ctx, current)
}

// render is the one place an account page becomes a response.
//
// `private, no-store` on all six: five of them carry a form a credential or an
// address is about to be typed into — two of those carry a live recovery token
// as well — and the sixth carries somebody's own address and display name. None
// of that belongs in a shared cache or on a disk. RenderPage is what applies
// it, along with the theme class and the version stamp every page carries.
func (p *accountPages) render(
	e *core.RequestEvent,
	title string,
	signedIn bool,
	nav []shell.NavLink,
	main web.Component,
) error {
	return RenderPage(e, http.StatusOK, title, NavState{SignedIn: signedIn, Nav: nav}, main)
}

// holdings is what deletion will destroy, as the confirmation states it.
//
// The labels are the kinds' own published segments, arriving as the keys of the
// counts the API answered with. Sorted, because ranging a map is not, and a
// confirmation that reordered itself between renders is one nobody can read
// twice.
func holdings(ctx context.Context, counts api.MeCounts) []settings.Holding {
	segments := make([]string, 0, len(counts))
	for segment := range counts {
		segments = append(segments, segment)
	}

	sort.Strings(segments)

	rendered := make([]settings.Holding, 0, len(segments))
	for _, segment := range segments {
		count := counts[segment]

		text := strconv.Itoa(count) + " " + segment
		if k, ok := kind.FromSegment(segment); ok {
			text = i18n.N(ctx, "kind."+k.Enum(), count)
		}

		rendered = append(rendered, settings.Holding{Text: text})
	}

	return rendered
}

// requireSession refuses a page that needs one to a caller who has none.
//
// 403 and not 404, exactly as the record pages are: contracts/pages.md's E2
// renders the sign-in prompt, because the existence of /settings is not
// information about anybody.
func requireSession(actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	return nil
}

// resolveLink asks whether the link this request opened still resolves, and to
// whom.
//
// A link that could NOT BE CHECKED is not a link that has expired, and the
// difference is the whole reason this is three cases rather than a boolean: a
// database that did not answer would otherwise tell somebody their link was
// dead on the strength of a failed read, and hide the outage behind a page that
// looks like it worked. That failure leaves here as an error and becomes the
// 500 it is.
//
// The empty account with no error is refused for the reason the service refuses
// it: an authenticator that resolved nothing has granted nothing, and treating
// it as a grant would offer a stranger's account a new password.
func (p *accountPages) resolveLink(
	e *core.RequestEvent,
	purpose identity.TokenPurpose,
) (domainidentity.User, bool, error) {
	user, err := p.deps.Links.Redeem(e.Request.Context(), purpose, e.Request.PathValue(PathToken))

	switch {
	case err == nil && user.ID != "":
		return user, true, nil
	case err == nil, errors.Is(err, identity.ErrInvalidToken):
		return domainidentity.User{}, false, nil
	default:
		return domainidentity.User{}, false, err
	}
}

// signedOutQuery reads one query parameter without inventing a vocabulary for
// the rest: an unrecognised value is treated as absent rather than refused,
// because a person following a stale link is not making a mistake worth a 400.
func queryIs(e *core.RequestEvent, name, want string) bool {
	return strings.EqualFold(e.Request.URL.Query().Get(name), want)
}
