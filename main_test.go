package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JackBraunYKT/go-project-278/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeStore struct {
	createLinkFunc func(context.Context, store.CreateLinkParams) (store.Link, error)
	deleteLinkFunc func(context.Context, int64) (int64, error)
	getLinkFunc    func(context.Context, int64) (store.Link, error)
	listLinksFunc  func(context.Context) ([]store.Link, error)
	updateLinkFunc func(context.Context, store.UpdateLinkParams) (store.Link, error)
}

func (f fakeStore) CreateLink(ctx context.Context, arg store.CreateLinkParams) (store.Link, error) {
	if f.createLinkFunc == nil {
		return store.Link{}, errors.New("unexpected CreateLink call")
	}

	return f.createLinkFunc(ctx, arg)
}

func (f fakeStore) DeleteLink(ctx context.Context, id int64) (int64, error) {
	if f.deleteLinkFunc == nil {
		return 0, errors.New("unexpected DeleteLink call")
	}

	return f.deleteLinkFunc(ctx, id)
}

func (f fakeStore) GetLink(ctx context.Context, id int64) (store.Link, error) {
	if f.getLinkFunc == nil {
		return store.Link{}, errors.New("unexpected GetLink call")
	}

	return f.getLinkFunc(ctx, id)
}

func (f fakeStore) ListLinks(ctx context.Context) ([]store.Link, error) {
	if f.listLinksFunc == nil {
		return nil, errors.New("unexpected ListLinks call")
	}

	return f.listLinksFunc(ctx)
}

func (f fakeStore) UpdateLink(ctx context.Context, arg store.UpdateLinkParams) (store.Link, error) {
	if f.updateLinkFunc == nil {
		return store.Link{}, errors.New("unexpected UpdateLink call")
	}

	return f.updateLinkFunc(ctx, arg)
}

func TestPingRouteReturnsPong(t *testing.T) {
	router := setupRouter(nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ping", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if response["message"] != "pong" {
		t.Fatalf("expected message %q, got %q", "pong", response["message"])
	}
}

func TestListLinksRouteReturnsLinks(t *testing.T) {
	t.Setenv("BASE_URL", "https://dev.short")

	router := setupRouter(fakeStore{
		listLinksFunc: func(context.Context) ([]store.Link, error) {
			return []store.Link{
				{ID: 1, OriginalUrl: "https://example.com/long-url", ShortName: "exmpl"},
				{ID: 2, OriginalUrl: "https://example.com/long-url2", ShortName: "exmpl2"},
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/links", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response []linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	expected := []linkResponse{
		{ID: 1, OriginalURL: "https://example.com/long-url", ShortName: "exmpl", ShortURL: "https://dev.short/r/exmpl"},
		{ID: 2, OriginalURL: "https://example.com/long-url2", ShortName: "exmpl2", ShortURL: "https://dev.short/r/exmpl2"},
	}

	if len(response) != len(expected) {
		t.Fatalf("expected %d links, got %d", len(expected), len(response))
	}

	for i := range expected {
		if response[i] != expected[i] {
			t.Fatalf("expected link %d to be %#v, got %#v", i, expected[i], response[i])
		}
	}
}

func TestCreateLinkRouteCreatesLinkWithProvidedShortName(t *testing.T) {
	var received store.CreateLinkParams
	router := setupRouter(fakeStore{
		createLinkFunc: func(_ context.Context, arg store.CreateLinkParams) (store.Link, error) {
			received = arg
			return store.Link{ID: 1, OriginalUrl: arg.OriginalUrl, ShortName: arg.ShortName}, nil
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/long-url","short_name":"exmpl"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	var response linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	expected := linkResponse{
		ID:          1,
		OriginalURL: "https://example.com/long-url",
		ShortName:   "exmpl",
		ShortURL:    "https://short.io/r/exmpl",
	}

	if response != expected {
		t.Fatalf("expected %#v, got %#v", expected, response)
	}

	if received.OriginalUrl != "https://example.com/long-url" {
		t.Fatalf("expected original url to be forwarded, got %q", received.OriginalUrl)
	}

	if received.ShortName != "exmpl" {
		t.Fatalf("expected short name to be forwarded, got %q", received.ShortName)
	}
}

func TestCreateLinkRouteGeneratesShortNameWhenMissing(t *testing.T) {
	router := setupRouter(fakeStore{
		createLinkFunc: func(_ context.Context, arg store.CreateLinkParams) (store.Link, error) {
			if arg.ShortName == "" {
				t.Fatal("expected generated short name")
			}

			return store.Link{ID: 1, OriginalUrl: arg.OriginalUrl, ShortName: arg.ShortName}, nil
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/long-url"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	var response linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if response.ShortName == "" {
		t.Fatal("expected response to include generated short name")
	}

	if response.ShortURL != "https://short.io/r/"+response.ShortName {
		t.Fatalf("expected short url to use generated short name, got %q", response.ShortURL)
	}
}

func TestCreateLinkRouteReturnsConflictWhenShortNameAlreadyExists(t *testing.T) {
	router := setupRouter(fakeStore{
		createLinkFunc: func(context.Context, store.CreateLinkParams) (store.Link, error) {
			return store.Link{}, &pgconn.PgError{Code: "23505"}
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/long-url","short_name":"exmpl"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusConflict, "short_name already exists")
}

func TestGetLinkRouteReturnsLink(t *testing.T) {
	router := setupRouter(fakeStore{
		getLinkFunc: func(_ context.Context, id int64) (store.Link, error) {
			if id != 7 {
				t.Fatalf("expected id 7, got %d", id)
			}

			return store.Link{ID: 7, OriginalUrl: "https://example.com/long-url", ShortName: "exmpl"}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/links/7", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	expected := linkResponse{
		ID:          7,
		OriginalURL: "https://example.com/long-url",
		ShortName:   "exmpl",
		ShortURL:    "https://short.io/r/exmpl",
	}

	if response != expected {
		t.Fatalf("expected %#v, got %#v", expected, response)
	}
}

func TestUpdateLinkRouteUpdatesLink(t *testing.T) {
	var received store.UpdateLinkParams
	router := setupRouter(fakeStore{
		updateLinkFunc: func(_ context.Context, arg store.UpdateLinkParams) (store.Link, error) {
			received = arg
			return store.Link{ID: arg.ID, OriginalUrl: arg.OriginalUrl, ShortName: arg.ShortName}, nil
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/updated","short_name":"updated"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/9", body)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if received.ID != 9 {
		t.Fatalf("expected id 9, got %d", received.ID)
	}

	if received.OriginalUrl != "https://example.com/updated" {
		t.Fatalf("expected original url to be forwarded, got %q", received.OriginalUrl)
	}

	if received.ShortName != "updated" {
		t.Fatalf("expected short name to be forwarded, got %q", received.ShortName)
	}

	var response linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if response.ShortURL != "https://short.io/r/updated" {
		t.Fatalf("expected short url to use updated short name, got %q", response.ShortURL)
	}
}

func TestDeleteLinkRouteDeletesLink(t *testing.T) {
	var receivedID int64
	router := setupRouter(fakeStore{
		deleteLinkFunc: func(_ context.Context, id int64) (int64, error) {
			receivedID = id
			return 1, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/links/11", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if recorder.Body.String() != "" {
		t.Fatalf("expected empty body, got %q", recorder.Body.String())
	}

	if receivedID != 11 {
		t.Fatalf("expected id 11, got %d", receivedID)
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	router := setupRouter(nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusNotFound, "not found")
}

func TestGetLinkRouteReturnsNotFoundWhenLinkDoesNotExist(t *testing.T) {
	router := setupRouter(fakeStore{
		getLinkFunc: func(context.Context, int64) (store.Link, error) {
			return store.Link{}, pgx.ErrNoRows
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/links/999", nil)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusNotFound, "link not found")
}

func TestUpdateLinkRouteReturnsNotFoundWhenLinkDoesNotExist(t *testing.T) {
	router := setupRouter(fakeStore{
		updateLinkFunc: func(context.Context, store.UpdateLinkParams) (store.Link, error) {
			return store.Link{}, pgx.ErrNoRows
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/updated","short_name":"updated"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/999", body)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusNotFound, "link not found")
}

func TestDeleteLinkRouteReturnsNotFoundWhenLinkDoesNotExist(t *testing.T) {
	router := setupRouter(fakeStore{
		deleteLinkFunc: func(context.Context, int64) (int64, error) {
			return 0, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/links/999", nil)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusNotFound, "link not found")
}

func TestSentryClientOptionsUseDSNFromEnvironment(t *testing.T) {
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")

	options := sentryClientOptionsFromEnv()

	if options.Dsn != "https://public@example.com/1" {
		t.Fatalf("expected Sentry DSN from environment, got %q", options.Dsn)
	}
}

func TestDatabaseURLFromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/app?sslmode=disable")

	databaseURL := databaseURLFromEnv()

	if databaseURL != "postgres://user:password@localhost:5432/app?sslmode=disable" {
		t.Fatalf("expected database url from environment, got %q", databaseURL)
	}
}

func TestLoadEnvFileIgnoresMissingFile(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), ".env")

	if err := loadEnvFile(missingFile); err != nil {
		t.Fatalf("expected missing env file to be ignored, got %v", err)
	}
}

func assertJSONError(t *testing.T, recorder *httptest.ResponseRecorder, status int, message string) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("expected status %d, got %d", status, recorder.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if response["error"] != message {
		t.Fatalf("expected error %q, got %q", message, response["error"])
	}
}
