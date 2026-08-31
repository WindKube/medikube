package identitytest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
)

// The identifiers and the credential this package's fakes answer for. They are
// its own and not the seeded fixture's: nothing here reaches a database, and a
// fake that borrowed a real id would read as though it did.
const (
	// Password is what a seeded fake account signs in with.
	//
	//nolint:gosec // an in-memory fake's credential, never a real one
	Password = "a-perfectly-ordinary-passphrase"

	// Email and Name are the account a service test signs in as.
	Email = "person@example.test"
	Name  = "A Person"

	// StrangerEmail belongs to nobody, so that every "an address with no
	// account" assertion has an address to make it with.
	StrangerEmail = "nobody@example.test"
)

// epoch is where the fake clock starts. Every reading advances it by a
// millisecond, so two rows never share an instant and the order a test asserts
// is the order the service wrote them in.
var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

// Repository is the in-memory identity.Repository: the second implementation
// Principle I requires, and the one the service's own tests run against.
//
// It holds the credentials as well as the rows, because identity.Repository's
// Create stores both in one act — FR-003's "no partially created account" is a
// property the fake has to have too, or every service test above it would be
// passing against something more forgiving than the store.
//
// It enforces the case-insensitive uniqueness of an address exactly as
// idx_users_email_lower does, which is the property T191 asserts against a real
// instance and identitytest.RunRepositoryContract asserts against both.
type Repository struct {
	mu sync.Mutex

	rows        map[string]domainidentity.User
	credentials map[string]string

	// generation is the record's token key, counted rather than spelled. It
	// moves on every credential change and on every explicit end-of-session,
	// which is what makes a token minted before one of those stop resolving.
	generation map[string]int

	next  int
	clock time.Time

	calls  []string
	writes []string
	err    error
}

func NewRepository() *Repository {
	return &Repository{
		rows:        make(map[string]domainidentity.User),
		credentials: make(map[string]string),
		generation:  make(map[string]int),
		clock:       epoch,
	}
}

// Fail makes every call this error, which is the case a service must not turn
// into a refusal: a store that could not answer has refused nobody.
func (r *Repository) Fail(err error) *Repository {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err

	return r
}

// Calls is every method reached since the last Forget, in order.
func (r *Repository) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

// Writes is the subset that changed something.
func (r *Repository) Writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.writes)
}

// Forget clears the journal and nothing else, so a test can seed through the
// repository and still assert what the call under test touched.
func (r *Repository) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls, r.writes = nil, nil
}

// Stored is the row as it is now, for a test that asserts a write did or did
// not happen. It bypasses the journal.
func (r *Repository) Stored(id string) (domainidentity.User, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.rows[id]

	return user, exists
}

func (r *Repository) Create(
	_ context.Context,
	draft domainidentity.User,
	password string,
) (domainidentity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")

	if r.err != nil {
		return domainidentity.User{}, r.err
	}

	if _, taken := r.byEmail(draft.Email); taken {
		return domainidentity.User{}, fmt.Errorf("identitytest: that address already has an account: %w", domain.ErrConflict)
	}

	r.writes = append(r.writes, "create")
	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	draft.ID = fmt.Sprintf("mkfakeuser%05d", r.next)
	draft.CreatedAt = r.clock
	draft.UpdatedAt = r.clock

	r.rows[draft.ID] = draft
	r.credentials[draft.ID] = password
	r.generation[draft.ID] = 1

	return draft, nil
}

func (r *Repository) Get(_ context.Context, id string) (domainidentity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")

	if r.err != nil {
		return domainidentity.User{}, r.err
	}

	user, exists := r.rows[id]
	if !exists {
		return domainidentity.User{}, fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	return user, nil
}

func (r *Repository) FindByEmail(_ context.Context, email string) (domainidentity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "find_by_email")

	if r.err != nil {
		return domainidentity.User{}, r.err
	}

	user, found := r.byEmail(email)
	if !found {
		return domainidentity.User{}, fmt.Errorf("identitytest: no account for that address: %w", domain.ErrNotFound)
	}

	return user, nil
}

func (r *Repository) Update(_ context.Context, user domainidentity.User) (domainidentity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	if r.err != nil {
		return domainidentity.User{}, r.err
	}

	current, exists := r.rows[user.ID]
	if !exists {
		return domainidentity.User{}, fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	// The address is the sign-in identity and moving it would move the account
	// out from under the unique index; a second account already holding it is a
	// conflict, exactly as the index would report.
	if other, taken := r.byEmail(user.Email); taken && other.ID != user.ID {
		return domainidentity.User{}, fmt.Errorf("identitytest: that address already has an account: %w", domain.ErrConflict)
	}

	r.writes = append(r.writes, "update")
	r.clock = r.clock.Add(time.Millisecond)

	user.CreatedAt = current.CreatedAt
	user.UpdatedAt = r.clock

	r.rows[user.ID] = user

	return user, nil
}

func (r *Repository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "delete")

	if r.err != nil {
		return r.err
	}

	if _, exists := r.rows[id]; !exists {
		return fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	r.writes = append(r.writes, "delete")

	delete(r.rows, id)
	delete(r.credentials, id)
	delete(r.generation, id)

	return nil
}

// byEmail is the LOWER(email) unique index, in a map. It must be called with
// the lock held.
func (r *Repository) byEmail(email string) (domainidentity.User, bool) {
	wanted := strings.ToLower(strings.TrimSpace(email))
	if wanted == "" {
		return domainidentity.User{}, false
	}

	for _, row := range r.rows {
		if strings.ToLower(row.Email) == wanted {
			return row, true
		}
	}

	return domainidentity.User{}, false
}

// The fixed dummy credential, and the whole of research D-17 in this fake.
//
// It is a constant and not a sampled row, because PocketBase's own
// dummyPasswordCheck picks any existing record and RETURNS EARLY when the query
// finds none — which restores the whole oracle on an empty table. A fixed value
// has no such branch.
//
//nolint:gosec // a dummy that no account can hold, which is the point
const dummyCredential = "\x00identitytest: the credential no account has"

// Authenticator is the in-memory identity.Authenticator, and it carries the
// counting seam T202 asserts through.
//
// Every refusal costs exactly one comparison, whether or not the address has an
// account, and Comparisons/DummyComparisons are how a test says so without a
// clock. That is the mechanism assertion research D-17 asks for: the latency is
// not deterministic and the count is.
type Authenticator struct {
	mu sync.Mutex

	repository *Repository

	comparisons int
	dummies     int

	calls []string
	err   error
}

func NewAuthenticator(repository *Repository) *Authenticator {
	return &Authenticator{repository: repository}
}

// Fail makes every call this error, which is a credential that could not be
// checked rather than one that did not match.
func (a *Authenticator) Fail(err error) *Authenticator {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.err = err

	return a
}

// Comparisons is how many times a password has been compared against a stored
// credential, the dummy included.
func (a *Authenticator) Comparisons() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.comparisons
}

// DummyComparisons is how many of those were against the fixed dummy — the
// comparison an address with no account still pays for.
func (a *Authenticator) DummyComparisons() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.dummies
}

// Calls is every method reached since the last Forget, in order.
func (a *Authenticator) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.calls)
}

func (a *Authenticator) Forget() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.calls, a.comparisons, a.dummies = nil, 0, 0
}

func (a *Authenticator) Authenticate(
	ctx context.Context,
	email, password string,
) (domainidentity.User, error) {
	a.mu.Lock()
	a.calls = append(a.calls, "authenticate")
	failure := a.err
	a.mu.Unlock()

	if failure != nil {
		return domainidentity.User{}, failure
	}

	user, err := a.repository.FindByEmail(ctx, email)
	if err != nil {
		// The equalisation. An address with no account pays for the same
		// comparison a wrong password does, so that the refusal cannot be told
		// apart by how long it took (research D-17).
		a.compare(dummyCredential, password, true)

		return domainidentity.User{}, refusedCredential()
	}

	if !a.compare(a.credential(user.ID), password, false) {
		return domainidentity.User{}, refusedCredential()
	}

	return user, nil
}

func (a *Authenticator) Verify(_ context.Context, userID, password string) error {
	a.mu.Lock()
	a.calls = append(a.calls, "verify")
	failure := a.err
	a.mu.Unlock()

	if failure != nil {
		return failure
	}

	stored, exists := a.repository.credentialOf(userID)
	if !exists {
		a.compare(dummyCredential, password, true)

		return refusedCredential()
	}

	if !a.compare(stored, password, false) {
		return refusedCredential()
	}

	return nil
}

func (a *Authenticator) SetPassword(_ context.Context, userID, password string) error {
	a.mu.Lock()
	a.calls = append(a.calls, "set_password")
	failure := a.err
	a.mu.Unlock()

	if failure != nil {
		return failure
	}

	// One write: the credential and the token key move together, which is what
	// makes every session issued before the change stop working (FR-010).
	if !a.repository.setCredential(userID, password) {
		return fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	return nil
}

func (a *Authenticator) EndSessions(_ context.Context, userID string) error {
	a.mu.Lock()
	a.calls = append(a.calls, "end_sessions")
	failure := a.err
	a.mu.Unlock()

	if failure != nil {
		return failure
	}

	if !a.repository.rotate(userID) {
		return fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	return nil
}

func (a *Authenticator) Redeem(
	_ context.Context,
	purpose identity.TokenPurpose,
	token string,
) (domainidentity.User, error) {
	a.mu.Lock()
	a.calls = append(a.calls, "redeem")
	failure := a.err
	a.mu.Unlock()

	if failure != nil {
		return domainidentity.User{}, failure
	}

	wanted, userID, generation, parsed := parseToken(token)
	if !parsed || wanted != purpose {
		return domainidentity.User{}, identity.ErrInvalidToken
	}

	user, exists := a.repository.Stored(userID)
	if !exists || a.repository.generationOf(userID) != generation {
		// The generation having moved is "already used": the same rotation that
		// ended the sessions is what spends the link.
		return domainidentity.User{}, identity.ErrInvalidToken
	}

	return user, nil
}

// Token mints a link token as the mailer would, for the contract suite and for
// the service's recovery tests.
func (a *Authenticator) Token(purpose identity.TokenPurpose, userID string) (string, error) {
	if _, exists := a.repository.Stored(userID); !exists {
		return "", fmt.Errorf("identitytest: no account with that id: %w", domain.ErrNotFound)
	}

	return string(purpose) + ":" + userID + ":" + strconv.Itoa(a.repository.generationOf(userID)), nil
}

func parseToken(token string) (identity.TokenPurpose, string, int, bool) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return "", "", 0, false
	}

	generation, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", "", 0, false
	}

	return identity.TokenPurpose(parts[0]), parts[1], generation, true
}

// compare is the seam. Every refusal path goes through it exactly once, and the
// counters are what a test reads instead of a stopwatch.
func (a *Authenticator) compare(stored, supplied string, dummy bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.comparisons++

	if dummy {
		a.dummies++
	}

	return stored == supplied
}

func (a *Authenticator) credential(userID string) string {
	stored, _ := a.repository.credentialOf(userID)

	return stored
}

// refusedCredential is the ONE refusal every failed check answers with, so that
// an unknown address and a wrong password cannot be told apart by the error
// either (FR-005).
func refusedCredential() error {
	return fmt.Errorf("identitytest: that address and password do not match an account: %w", domain.ErrUnauthenticated)
}

func (r *Repository) credentialOf(userID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored, exists := r.credentials[userID]

	return stored, exists
}

func (r *Repository) generationOf(userID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.generation[userID]
}

func (r *Repository) setCredential(userID, password string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rows[userID]; !exists {
		return false
	}

	r.credentials[userID] = password
	r.generation[userID]++

	return true
}

func (r *Repository) rotate(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rows[userID]; !exists {
		return false
	}

	r.generation[userID]++

	return true
}

// Mailer is the in-memory identity.Mailer. It records the account each message
// was for and never an address, because an address is what the service is not
// allowed to hand it (FR-077).
type Mailer struct {
	mu sync.Mutex

	resets        []string
	verifications []string
	err           error
}

func NewMailer() *Mailer { return &Mailer{} }

// Fail makes every send this error, which is the case a recovery request must
// not let change its answer.
func (m *Mailer) Fail(err error) *Mailer {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.err = err

	return m
}

// Resets is the account id of every recovery message sent, in order.
func (m *Mailer) Resets() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.resets)
}

// Verifications is the account id of every confirmation message sent.
func (m *Mailer) Verifications() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.verifications)
}

func (m *Mailer) SendPasswordReset(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.resets = append(m.resets, userID)

	return nil
}

func (m *Mailer) SendVerification(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.verifications = append(m.verifications, userID)

	return nil
}

// Auditor collects the rows the service writes.
type Auditor struct {
	mu sync.Mutex

	events []audit.Event
	err    error
}

func NewAuditor() *Auditor { return &Auditor{} }

// Fail makes the trail unwritable, which is the case the service must not turn
// into a different answer for the caller.
func (a *Auditor) Fail(err error) *Auditor {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.err = err

	return a
}

func (a *Auditor) Events() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.events)
}

// Actions is the action of every row written, in order, which is what an audit
// coverage assertion reads.
func (a *Auditor) Actions() []audit.Action {
	a.mu.Lock()
	defer a.mu.Unlock()

	found := make([]audit.Action, 0, len(a.events))
	for _, event := range a.events {
		found = append(found, event.Action)
	}

	return found
}

func (a *Auditor) Forget() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = nil
}

func (a *Auditor) Record(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.err != nil {
		return a.err
	}

	a.events = append(a.events, event)

	return nil
}

// Clock is the fake identity.Clock. It advances a millisecond per reading, so
// two rows never share an instant and the order a test asserts is the order the
// service wrote them in.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

func NewClock() *Clock { return &Clock{now: epoch} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(time.Millisecond)

	return c.now
}
