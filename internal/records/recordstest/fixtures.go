package recordstest

// Fixture is one kind's create-body builders: the least a record needs to be
// valid, and every optional field populated too. Both build the shape
// records.Schema.NewCreate mints — what records.Service.Create accepts — so
// RepositoryContract, KindContract and `medikube seed` build every fixture
// record from this one pair rather than each inventing its own.
type Fixture struct {
	// Minimal is the least a create body needs: whatever the kind's own
	// Validate requires, and nothing else. A kind whose default sort orders
	// by a primary date that Minimal leaves unset is what
	// RepositoryContractOptions.NullPrimaryDate proves sorts last.
	Minimal func(patientID string) any

	// Full is every field the kind's create body declares, populated.
	Full func(patientID string) any
}
