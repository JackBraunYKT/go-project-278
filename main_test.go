package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JackBraunYKT/go-project-278/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeStore struct {
	countLinkVisitsFunc    func(context.Context) (int64, error)
	countLinksFunc         func(context.Context) (int64, error)
	createLinkFunc         func(context.Context, store.CreateLinkParams) (store.Link, error)
	createLinkVisitFunc    func(context.Context, store.CreateLinkVisitParams) (store.LinkVisit, error)
	deleteLinkFunc         func(context.Context, int64) (int64, error)
	getLinkFunc            func(context.Context, int64) (store.Link, error)
	getLinkByShortNameFunc func(context.Context, string) (store.Link, error)
	listLinkVisitsFunc     func(context.Context) ([]store.LinkVisit, error)
	listLinkVisitsPageFunc func(context.Context, store.ListLinkVisitsPageParams) ([]store.LinkVisit, error)
	listLinksFunc          func(context.Context) ([]store.Link, error)
	listLinksPageFunc      func(context.Context, store.ListLinksPageParams) ([]store.Link, error)
	updateLinkFunc         func(context.Context, store.UpdateLinkParams) (store.Link, error)
}

func (f fakeStore) CountLinkVisits(ctx context.Context) (int64, error) {
	if f.countLinkVisitsFunc == nil {
		return 0, errors.New("unexpected CountLinkVisits call")
	}

	return f.countLinkVisitsFunc(ctx)
}

func (f fakeStore) CountLinks(ctx context.Context) (int64, error) {
	if f.countLinksFunc == nil {
		return 0, errors.New("unexpected CountLinks call")
	}

	return f.countLinksFunc(ctx)
}

func (f fakeStore) CreateLink(ctx context.Context, arg store.CreateLinkParams) (store.Link, error) {
	if f.createLinkFunc == nil {
		return store.Link{}, errors.New("unexpected CreateLink call")
	}

	return f.createLinkFunc(ctx, arg)
}

func (f fakeStore) CreateLinkVisit(ctx context.Context, arg store.CreateLinkVisitParams) (store.LinkVisit, error) {
	if f.createLinkVisitFunc == nil {
		return store.LinkVisit{}, errors.New("unexpected CreateLinkVisit call")
	}

	return f.createLinkVisitFunc(ctx, arg)
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

func (f fakeStore) GetLinkByShortName(ctx context.Context, shortName string) (store.Link, error) {
	if f.getLinkByShortNameFunc == nil {
		return store.Link{}, errors.New("unexpected GetLinkByShortName call")
	}

	return f.getLinkByShortNameFunc(ctx, shortName)
}

func (f fakeStore) ListLinkVisits(ctx context.Context) ([]store.LinkVisit, error) {
	if f.listLinkVisitsFunc == nil {
		return nil, errors.New("unexpected ListLinkVisits call")
	}

	return f.listLinkVisitsFunc(ctx)
}

func (f fakeStore) ListLinkVisitsPage(ctx context.Context, arg store.ListLinkVisitsPageParams) ([]store.LinkVisit, error) {
	if f.listLinkVisitsPageFunc == nil {
		return nil, errors.New("unexpected ListLinkVisitsPage call")
	}

	return f.listLinkVisitsPageFunc(ctx, arg)
}

func (f fakeStore) ListLinks(ctx context.Context) ([]store.Link, error) {
	if f.listLinksFunc == nil {
		return nil, errors.New("unexpected ListLinks call")
	}

	return f.listLinksFunc(ctx)
}

func (f fakeStore) ListLinksPage(ctx context.Context, arg store.ListLinksPageParams) ([]store.Link, error) {
	if f.listLinksPageFunc == nil {
		return nil, errors.New("unexpected ListLinksPage call")
	}

	return f.listLinksPageFunc(ctx, arg)
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

func TestRouterTrustsCloudflarePlatform(t *testing.T) {
	router := setupRouter(nil)

	if router.TrustedPlatform != gin.PlatformCloudflare {
		t.Fatalf("expected trusted platform %q, got %q", gin.PlatformCloudflare, router.TrustedPlatform)
	}
}

func TestAPIRouteAllowsFrontendOrigin(t *testing.T) {
	router := setupRouter(fakeStore{
		listLinksFunc: func(context.Context) ([]store.Link, error) {
			return []store.Link{}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	request.Header.Set("Origin", "http://localhost:5173")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if origin := recorder.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:5173" {
		t.Fatalf("expected frontend origin to be allowed, got %q", origin)
	}

	if exposedHeaders := recorder.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(exposedHeaders, "Content-Range") {
		t.Fatalf("expected Content-Range to be exposed, got %q", exposedHeaders)
	}
}

func TestPreflightAllowsFrontendRequests(t *testing.T) {
	router := setupRouter(nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/links", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Content-Type")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	if origin := recorder.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:5173" {
		t.Fatalf("expected frontend origin to be allowed, got %q", origin)
	}

	if methods := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, http.MethodPost) {
		t.Fatalf("expected POST method to be allowed, got %q", methods)
	}

	if headers := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(headers, "Content-Type") {
		t.Fatalf("expected Content-Type header to be allowed, got %q", headers)
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

func TestRedirectRouteRedirectsToOriginalURLAndRecordsVisit(t *testing.T) {
	var receivedVisit store.CreateLinkVisitParams
	router := setupRouter(fakeStore{
		getLinkByShortNameFunc: func(_ context.Context, shortName string) (store.Link, error) {
			if shortName != "exmpl" {
				t.Fatalf("expected short name %q, got %q", "exmpl", shortName)
			}

			return store.Link{ID: 7, OriginalUrl: "https://example.com/long-url", ShortName: shortName}, nil
		},
		createLinkVisitFunc: func(_ context.Context, arg store.CreateLinkVisitParams) (store.LinkVisit, error) {
			receivedVisit = arg
			return store.LinkVisit{ID: 5, LinkID: arg.LinkID, Ip: arg.Ip, UserAgent: arg.UserAgent, Referer: arg.Referer, Status: arg.Status}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/r/exmpl", nil)
	request.Header.Set("CF-Connecting-IP", "203.0.113.10")
	request.Header.Set("User-Agent", "curl/8.5.0")
	request.Header.Set("Referer", "https://ref.example/path")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, recorder.Code)
	}

	if location := recorder.Header().Get("Location"); location != "https://example.com/long-url" {
		t.Fatalf("expected redirect location %q, got %q", "https://example.com/long-url", location)
	}

	if receivedVisit.LinkID != 7 {
		t.Fatalf("expected link id 7, got %d", receivedVisit.LinkID)
	}

	if receivedVisit.Ip != "203.0.113.10" {
		t.Fatalf("expected Cloudflare client IP, got %q", receivedVisit.Ip)
	}

	if receivedVisit.UserAgent != "curl/8.5.0" {
		t.Fatalf("expected user agent to be recorded, got %q", receivedVisit.UserAgent)
	}

	if receivedVisit.Referer != "https://ref.example/path" {
		t.Fatalf("expected referer to be recorded, got %q", receivedVisit.Referer)
	}

	if receivedVisit.Status != http.StatusFound {
		t.Fatalf("expected status %d to be recorded, got %d", http.StatusFound, receivedVisit.Status)
	}
}

func TestRedirectRouteReturnsNotFoundWhenShortNameDoesNotExist(t *testing.T) {
	router := setupRouter(fakeStore{
		getLinkByShortNameFunc: func(context.Context, string) (store.Link, error) {
			return store.Link{}, pgx.ErrNoRows
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/r/missing", nil)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusNotFound, "link not found")
}

func TestListLinksRouteReturnsRequestedRange(t *testing.T) {
	t.Setenv("BASE_URL", "https://dev.short")

	page := make([]store.Link, 5)
	for i := range page {
		id := int64(i + 6)
		page[i] = store.Link{
			ID:          id,
			OriginalUrl: "https://example.com/link",
			ShortName:   "short",
		}
	}

	router := setupRouter(fakeStore{
		countLinksFunc: func(context.Context) (int64, error) {
			return 12, nil
		},
		listLinksPageFunc: func(_ context.Context, arg store.ListLinksPageParams) ([]store.Link, error) {
			if arg.PageOffset != 5 {
				t.Fatalf("expected page offset 5, got %d", arg.PageOffset)
			}

			if arg.PageLimit != 5 {
				t.Fatalf("expected page limit 5, got %d", arg.PageLimit)
			}

			return page, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/links?range=[5,9]", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if contentRange := recorder.Header().Get("Content-Range"); contentRange != "links 5-9/12" {
		t.Fatalf("expected Content-Range %q, got %q", "links 5-9/12", contentRange)
	}

	var response []linkResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if len(response) != 5 {
		t.Fatalf("expected 5 links, got %d", len(response))
	}

	if response[0].ID != 6 {
		t.Fatalf("expected first returned link id to be 6, got %d", response[0].ID)
	}

	if response[len(response)-1].ID != 10 {
		t.Fatalf("expected last returned link id to be 10, got %d", response[len(response)-1].ID)
	}
}

func TestListLinkVisitsRouteReturnsVisits(t *testing.T) {
	createdAt := time.Date(2025, 10, 31, 13, 1, 43, 0, time.UTC)
	router := setupRouter(fakeStore{
		listLinkVisitsFunc: func(context.Context) ([]store.LinkVisit, error) {
			return []store.LinkVisit{
				{
					ID:        5,
					LinkID:    1,
					CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
					Ip:        "172.18.0.1",
					UserAgent: "curl/8.5.0",
					Referer:   "https://ref.example/path",
					Status:    http.StatusFound,
				},
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/link_visits", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response []linkVisitResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	expected := []linkVisitResponse{
		{
			ID:        5,
			LinkID:    1,
			CreatedAt: createdAt,
			IP:        "172.18.0.1",
			UserAgent: "curl/8.5.0",
			Status:    http.StatusFound,
		},
	}

	if len(response) != len(expected) {
		t.Fatalf("expected %d visits, got %d", len(expected), len(response))
	}

	if response[0] != expected[0] {
		t.Fatalf("expected visit %#v, got %#v", expected[0], response[0])
	}
}

func TestListLinkVisitsRouteReturnsRequestedRange(t *testing.T) {
	page := []store.LinkVisit{
		{ID: 11, LinkID: 1, Ip: "172.18.0.1", UserAgent: "curl/8.5.0", Status: http.StatusFound},
		{ID: 12, LinkID: 1, Ip: "172.18.0.2", UserAgent: "curl/8.5.0", Status: http.StatusFound},
	}

	router := setupRouter(fakeStore{
		countLinkVisitsFunc: func(context.Context) (int64, error) {
			return 357, nil
		},
		listLinkVisitsPageFunc: func(_ context.Context, arg store.ListLinkVisitsPageParams) ([]store.LinkVisit, error) {
			if arg.PageOffset != 10 {
				t.Fatalf("expected page offset 10, got %d", arg.PageOffset)
			}

			if arg.PageLimit != 11 {
				t.Fatalf("expected page limit 11, got %d", arg.PageLimit)
			}

			return page, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/link_visits", nil)
	request.Header.Set("Range", "[10, 20]")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if acceptRanges := recorder.Header().Get("Accept-Ranges"); acceptRanges != "link_visits" {
		t.Fatalf("expected Accept-Ranges %q, got %q", "link_visits", acceptRanges)
	}

	if contentRange := recorder.Header().Get("Content-Range"); contentRange != "link_visits 10-11/357" {
		t.Fatalf("expected Content-Range %q, got %q", "link_visits 10-11/357", contentRange)
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

func TestCreateLinkRouteTrimsPayloadBeforeValidation(t *testing.T) {
	var received store.CreateLinkParams
	router := setupRouter(fakeStore{
		createLinkFunc: func(_ context.Context, arg store.CreateLinkParams) (store.Link, error) {
			received = arg
			return store.Link{ID: 1, OriginalUrl: arg.OriginalUrl, ShortName: arg.ShortName}, nil
		},
	})

	body := bytes.NewBufferString(`{"original_url":" https://example.com/long-url ","short_name":" exmpl "}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	if received.OriginalUrl != "https://example.com/long-url" {
		t.Fatalf("expected original url to be trimmed, got %q", received.OriginalUrl)
	}

	if received.ShortName != "exmpl" {
		t.Fatalf("expected short name to be trimmed, got %q", received.ShortName)
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

func TestCreateLinkRouteReturnsBadRequestForInvalidJSON(t *testing.T) {
	router := setupRouter(fakeStore{})

	body := bytes.NewBufferString(`{"original_url":`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusBadRequest, "invalid request")
}

func TestCreateLinkRouteReturnsValidationErrors(t *testing.T) {
	router := setupRouter(fakeStore{})

	body := bytes.NewBufferString(`{"original_url":"not a url","short_name":"ab"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	assertValidationErrors(t, recorder, http.StatusUnprocessableEntity, map[string]string{
		"original_url": "Key: 'createLinkPayload.original_url' Error:Field validation for 'original_url' failed on the 'url' tag",
		"short_name":   "Key: 'createLinkPayload.short_name' Error:Field validation for 'short_name' failed on the 'min' tag",
	})
}

func TestCreateLinkRouteReturnsValidationErrorWhenOriginalURLMissing(t *testing.T) {
	router := setupRouter(fakeStore{})

	body := bytes.NewBufferString(`{"short_name":"exmpl"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/links", body)

	router.ServeHTTP(recorder, request)

	assertValidationErrors(t, recorder, http.StatusUnprocessableEntity, map[string]string{
		"original_url": "Key: 'createLinkPayload.original_url' Error:Field validation for 'original_url' failed on the 'required' tag",
	})
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

	assertValidationErrors(t, recorder, http.StatusUnprocessableEntity, map[string]string{
		"short_name": "short name already in use",
	})
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

func TestUpdateLinkRoutePreservesShortNameWhenMissing(t *testing.T) {
	var received store.UpdateLinkParams
	router := setupRouter(fakeStore{
		getLinkFunc: func(_ context.Context, id int64) (store.Link, error) {
			if id != 9 {
				t.Fatalf("expected id 9, got %d", id)
			}

			return store.Link{ID: id, OriginalUrl: "https://example.com/old", ShortName: "existing"}, nil
		},
		updateLinkFunc: func(_ context.Context, arg store.UpdateLinkParams) (store.Link, error) {
			received = arg
			return store.Link{ID: arg.ID, OriginalUrl: arg.OriginalUrl, ShortName: arg.ShortName}, nil
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/updated"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/9", body)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if received.ShortName != "existing" {
		t.Fatalf("expected existing short name to be preserved, got %q", received.ShortName)
	}
}

func TestUpdateLinkRouteReturnsBadRequestForInvalidJSON(t *testing.T) {
	router := setupRouter(fakeStore{})

	body := bytes.NewBufferString(`{"original_url":`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/9", body)

	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusBadRequest, "invalid request")
}

func TestUpdateLinkRouteReturnsValidationErrors(t *testing.T) {
	router := setupRouter(fakeStore{})

	body := bytes.NewBufferString(`{"original_url":"not a url","short_name":"ab"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/9", body)

	router.ServeHTTP(recorder, request)

	assertValidationErrors(t, recorder, http.StatusUnprocessableEntity, map[string]string{
		"original_url": "Key: 'updateLinkPayload.original_url' Error:Field validation for 'original_url' failed on the 'url' tag",
		"short_name":   "Key: 'updateLinkPayload.short_name' Error:Field validation for 'short_name' failed on the 'min' tag",
	})
}

func TestUpdateLinkRouteReturnsValidationErrorWhenShortNameAlreadyExists(t *testing.T) {
	router := setupRouter(fakeStore{
		updateLinkFunc: func(context.Context, store.UpdateLinkParams) (store.Link, error) {
			return store.Link{}, &pgconn.PgError{Code: "23505"}
		},
	})

	body := bytes.NewBufferString(`{"original_url":"https://example.com/updated","short_name":"updated"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/links/9", body)

	router.ServeHTTP(recorder, request)

	assertValidationErrors(t, recorder, http.StatusUnprocessableEntity, map[string]string{
		"short_name": "short name already in use",
	})
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

func assertValidationErrors(t *testing.T, recorder *httptest.ResponseRecorder, status int, expected map[string]string) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("expected status %d, got %d", status, recorder.Code)
	}

	var response struct {
		Errors map[string]string `json:"errors"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if len(response.Errors) != len(expected) {
		t.Fatalf("expected validation errors %#v, got %#v", expected, response.Errors)
	}

	for field, message := range expected {
		if response.Errors[field] != message {
			t.Fatalf("expected validation error %s=%q, got %q", field, message, response.Errors[field])
		}
	}
}
