package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedirect проверяет, что короткие ссылки перенаправляют и записывают детали посещения.
func TestRedirect(t *testing.T) {
	_, queries := setupTx(t)
	fixtureLinks, err := LoadLinkFixtures(t)
	require.NoError(t, err)
	err = SeedLinks(context.Background(), queries, fixtureLinks)
	require.NoError(t, err)
	router := SetupRouter(queries)
	links, err := queries.ListAllLinks(context.Background())
	require.NoError(t, err)
	link := links[0]

	req, err := http.NewRequest("GET", fmt.Sprintf("/r/%s", link.ShortName), nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "https://example.com")

	req.RemoteAddr = "203.0.113.10:12345"

	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMovedPermanently, w.Code)

	linkVisit, err := queries.GetLastLinkVisitByLinkID(context.Background(), link.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(http.StatusMovedPermanently), linkVisit.Status)
	assert.Equal(t, "test-agent", linkVisit.UserAgent)
	assert.Equal(t, "https://example.com", linkVisit.Referer)
	assert.Equal(t, "203.0.113.10", linkVisit.Ip)
}
