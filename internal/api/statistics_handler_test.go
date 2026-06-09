package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetLinkVisits проверяет, что эндпоинт статистики возвращает все добавленные посещения.
func TestGetLinkVisits(t *testing.T) {
	_, queries := setupTx(t)
	fixtureLinks, err := LoadLinkFixtures(t)
	require.NoError(t, err)

	err = SeedLinks(context.Background(), queries, fixtureLinks)
	require.NoError(t, err)

	fixtureLinkVisits, err := LoadLinkVisitFixtures(t)
	require.NoError(t, err)
	err = SeedLinkVisits(context.Background(), queries, fixtureLinkVisits)
	require.NoError(t, err)
	router := SetupRouter(queries)

	req, err := http.NewRequest("GET", "/api/link_visits", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []LinkVisitResponse

	err = json.Unmarshal(w.Body.Bytes(), &response)

	require.NoError(t, err)
	require.Len(t, response, 3)

	assert.Equal(t, fixtureLinkVisits[0].Referer, response[0].Referer)
	assert.Equal(t, fixtureLinkVisits[1].Referer, response[1].Referer)
	assert.Equal(t, fixtureLinkVisits[2].Referer, response[2].Referer)
}

// TestGetLinkVisitsWithPagination проверяет, что статистика учитывает range-запрос.
func TestGetLinkVisitsWithPagination(t *testing.T) {
	_, queries := setupTx(t)

	fixtureLinks, err := LoadLinkFixtures(t)
	require.NoError(t, err)

	err = SeedLinks(context.Background(), queries, fixtureLinks)
	require.NoError(t, err)

	fixtureLinkVisits, err := LoadLinkVisitFixtures(t)
	require.NoError(t, err)
	err = SeedLinkVisits(context.Background(), queries, fixtureLinkVisits)
	require.NoError(t, err)
	router := SetupRouter(queries)

	req, err := http.NewRequest("GET", "/api/link_visits?range=[1,2]", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "link_visits 1-1/3", w.Header().Get("Content-Range"))

	var response []LinkVisitResponse

	err = json.Unmarshal(w.Body.Bytes(), &response)

	require.NoError(t, err)

	require.Len(t, response, 1)

	assert.Equal(t, fixtureLinkVisits[1].Referer, response[0].Referer)
}
