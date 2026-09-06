package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
)

func TestAnUpdateOverDatastarCarriesTheDocumentTitle(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), `{"patient":"`+testsupport.AccountAPatientSelfID+`","name":"Before"}`)
	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)
	created := answer.medication(t)

	answer = caller.do(http.MethodPatch, recordURL(created.ID), `{"name":"After"}`, map[string]string{
		"Datastar-Request": "true",
		"If-Match":         web.ETag(storedVersion(t, caller, created.ID)),
	})
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	assert.Contains(t, answer.Body, shell.TitleElement("After"))
	assert.Contains(t, answer.Body, ids.RecordDetail(kind.Medication, created.ID))
	assert.Contains(t, answer.Body, ids.RecordForm(kind.Medication, created.ID))
}
