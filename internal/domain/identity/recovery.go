package identity

// DeleteConfirmationPhrase is what a person types to delete their own account
// (FR-013, contracts/account.md). It is spelled once, here, because the form
// that renders it, the handler that compares it and the test that types it must
// all mean the same seventeen characters — and a phrase that differed by a
// space between the form and the check would be a delete nobody could complete.
//
// The comparison is exact: no trimming, no case folding. A destructive,
// irreversible act asks for a deliberate act in return, and "delete my account"
// typed in lower case is a person who was not reading.
const DeleteConfirmationPhrase = "DELETE MY ACCOUNT"

// RecoveryStatus is the whole of what a recovery request answers
// (contracts/auth.md, `requestPasswordReset`).
//
// Its wording is deliberately about the request and not about the account:
// "sent if registered" is true whether or not anybody is registered, which is
// what makes it safe to say to a caller who is fishing for the difference.
const RecoveryStatus = "sent_if_registered"

// Acknowledgement is the answer to "send me a recovery message".
//
// FR-073 requires every such request to be answered identically whether or not
// an account exists, and this type is how that stops being a rule somebody has
// to remember: AcknowledgeRecovery takes no arguments, so there is nothing an
// account's existence could be carried in. A branch that wanted to answer
// differently would have to introduce a second constructor, which is a visible
// change rather than a forgotten one.
type Acknowledgement struct {
	Status string
}

// AcknowledgeRecovery is the one constructor, and its empty parameter list is
// the guarantee. See Acknowledgement.
func AcknowledgeRecovery() Acknowledgement {
	return Acknowledgement{Status: RecoveryStatus}
}
