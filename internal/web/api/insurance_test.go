package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// TestInsuranceCoverageAndContactRoundTripAsTypedObjects is T123: coverage
// and contact travel the wire as CoverageDTO/ContactDTO, not a raw map, so a
// draft built from a create body and rendered back out as a detail carries
// the same values it was decoded with.
func TestInsuranceCoverageAndContactRoundTripAsTypedObjects(t *testing.T) {
	t.Parallel()

	deductible := "500.00"
	coinsurance := 0.2

	create := &api.InsuranceCreate{
		Patient:     "pat0000000000001",
		Type:        "medical",
		Company:     "Acme Health",
		MemberName:  "A Person",
		MemberID:    "MEM-1",
		EffectiveOn: "2024-01-01",
		Coverage:    &api.CoverageDTO{Deductible: &deductible, CoinsurancePct: &coinsurance, Currency: "USD"},
		Contact:     &api.ContactDTO{Phone: "555-0100", Website: "https://example.test"},
	}

	entity, err := api.InsuranceCodec{}.Draft(create)
	require.NoError(t, err)

	require.NotNil(t, entity.Coverage.Deductible)
	assert.Equal(t, "500.00", entity.Coverage.Deductible.String())
	require.NotNil(t, entity.Coverage.CoinsurancePct)
	assert.InDelta(t, 0.2, *entity.Coverage.CoinsurancePct, 0.0001)
	assert.Equal(t, "555-0100", entity.Contact.Phone)
	assert.Equal(t, "https://example.test", entity.Contact.Website)

	detail, ok := api.InsuranceCodec{}.Detail(entity, nil).(*api.Insurance)
	require.True(t, ok)
	require.NotNil(t, detail.Coverage)
	require.NotNil(t, detail.Coverage.Deductible)
	assert.Equal(t, "500.00", *detail.Coverage.Deductible)
	require.NotNil(t, detail.Coverage.CoinsurancePct)
	assert.InDelta(t, 0.2, *detail.Coverage.CoinsurancePct, 0.0001)
	require.NotNil(t, detail.Contact)
	assert.Equal(t, "555-0100", detail.Contact.Phone)
}

// TestInsuranceCreateRejectsAnUnknownMember proves the generic decode path
// (not web.Decode directly, since the record body is minted through
// records.Schema.NewCreate) still refuses a member neither DTO declares.
func TestInsuranceCreateRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	valid, err := json.Marshal(&api.InsuranceCreate{
		Patient: "pat0000000000001", Type: "medical", Company: "Acme Health",
		MemberName: "A Person", MemberID: "MEM-1", EffectiveOn: "2024-01-01",
	})
	require.NoError(t, err)

	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(valid, &members))

	members["zzz_a_field_no_dto_declares"] = json.RawMessage(`"anything"`)

	tainted, err := json.Marshal(members)
	require.NoError(t, err)

	target := new(api.InsuranceCreate)
	decodeErr := web.DecodeBytes(tainted, target)
	require.Error(t, decodeErr)

	var invalid *domain.ValidationError
	require.ErrorAs(t, decodeErr, &invalid)

	var sawUnknownField bool
	for _, field := range invalid.Fields {
		if field.Code == domain.CodeUnknownField {
			sawUnknownField = true
		}
	}
	assert.True(t, sawUnknownField)
}

// TestInsuranceBasisReadsBackTheSummarysField is FR-046's basis mechanism at
// the DTO boundary: InsuranceBasis reads whatever InsuranceCodec.Summary
// already computed, rather than computing anything of its own.
func TestInsuranceBasisReadsBackTheSummarysField(t *testing.T) {
	t.Parallel()

	summary, ok := api.InsuranceCodec{}.Summary(clinical.Insurance{ID: "ins1"}, []string{"expiring"}).(*api.InsuranceSummary)
	require.True(t, ok)
	assert.Equal(t, []string{"expiring"}, summary.Basis)
	assert.Equal(t, []string{"expiring"}, api.InsuranceBasis(summary, records.Criteria{}))
}
