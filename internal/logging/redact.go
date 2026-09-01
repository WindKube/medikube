package logging

// Placeholder stands in for a value that must never reach the operational
// record (FR-038, FR-041).
const Placeholder = "[redacted]"

// Sensitive is a string that cannot be printed. Its String, GoString,
// MarshalText and MarshalJSON all return Placeholder, so a field of this type
// reaching a log line, an error message, a Sentry payload or a JSON body
// carries nothing. Reveal is the only way to the value, and it is a visible act
// at the call site.
//
// This is what constitution Principle VII means by structural: a type that
// leaks only when somebody writes Reveal, rather than a rule everyone has to
// remember. The empty value redacts too — branching on emptiness would turn the
// placeholder into a signal for whether a secret is set.
type Sensitive string

func (s Sensitive) String() string { return Placeholder }

func (s Sensitive) GoString() string { return Placeholder }

func (s Sensitive) MarshalText() ([]byte, error) { return []byte(Placeholder), nil }

func (s Sensitive) MarshalJSON() ([]byte, error) { return []byte(`"` + Placeholder + `"`), nil }

// Reveal returns the value. Every call is a deliberate decision to handle it.
func (s Sensitive) Reveal() string { return string(s) }
