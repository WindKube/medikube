package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain"
)

func TestSamePatient(t *testing.T) {
	t.Parallel()

	subject := "patient1"

	cases := []struct {
		name    string
		targets []PatientRef
		wantErr error
	}{
		{
			name:    "matching patients pass",
			targets: []PatientRef{{ID: "t1", PatientID: subject, Found: true}},
		},
		{
			name:    "a differing patient is refused",
			targets: []PatientRef{{ID: "t1", PatientID: "patient2", Found: true}},
			wantErr: domain.ErrNotFound,
		},
		{
			name:    "a non-existent target is refused identically",
			targets: []PatientRef{{ID: "t1", Found: false}},
			wantErr: domain.ErrNotFound,
		},
		{
			name:    "an unreachable target is refused identically",
			targets: []PatientRef{{ID: "t1", PatientID: subject, Found: false}},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "one bad target among good ones still refuses",
			targets: []PatientRef{
				{ID: "t1", PatientID: subject, Found: true},
				{ID: "t2", PatientID: "patient2", Found: true},
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := SamePatient(subject, tt.targets)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestLinkSetReplaceSetSemantics(t *testing.T) {
	t.Parallel()

	set := NewLinkSet("a", "b", "a")
	assert.Equal(t, 2, set.Len(), "a repeated id is one member")
	assert.True(t, set.Contains("a"))
	assert.True(t, set.Contains("b"))
	assert.False(t, set.Contains("c"))
}

func TestLinkSetEqualIsOrderAndRepetitionInvariant(t *testing.T) {
	t.Parallel()

	assert.True(t, NewLinkSet("a", "b").Equal(NewLinkSet("b", "a", "b")))
	assert.False(t, NewLinkSet("a", "b").Equal(NewLinkSet("a")))
	assert.False(t, NewLinkSet("a").Equal(NewLinkSet("b")))
}

// Re-adding an existing member is an idempotent no-op (FR-056): the set
// submitted a second time equals the set already stored.
func TestReAddingAnExistingMemberIsIdempotent(t *testing.T) {
	t.Parallel()

	stored := NewLinkSet("a", "b")
	resubmitted := NewLinkSet("a", "b", "a")

	assert.True(t, stored.Equal(resubmitted))
}
