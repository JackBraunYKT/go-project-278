package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/JackBraunYKT/go-project-278/internal/database"
	"github.com/JackBraunYKT/go-project-278/internal/store"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/joho/godotenv"
)

const (
	defaultBaseURL       = "https://short.io"
	frontendOrigin       = "http://localhost:5173"
	generatedNameBytes   = 6
	linkVisitsRangeUnit  = "link_visits"
	linksRangeUnit       = "links"
	maxLinksRangeLimit   = int64(1<<31 - 1)
	maxShortNameAttempts = 5
)

type createLinkPayload struct {
	OriginalURL string `json:"original_url" binding:"required,url"`
	ShortName   string `json:"short_name" binding:"omitempty,min=3,max=32"`
}

type updateLinkPayload struct {
	OriginalURL string `json:"original_url" binding:"required,url"`
	ShortName   string `json:"short_name" binding:"omitempty,min=3,max=32"`
}

type linkResponse struct {
	ID          int64  `json:"id"`
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
	ShortURL    string `json:"short_url"`
}

type linkVisitResponse struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"link_id"`
	CreatedAt time.Time `json:"created_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Status    int32     `json:"status"`
}

type linksRange struct {
	start int64
	end   int64
}

func init() {
	configureValidator()
}

func configureValidator() {
	if validate, ok := binding.Validator.Engine().(*validator.Validate); ok {
		validate.RegisterTagNameFunc(jsonTagName)
	}
}

func jsonTagName(field reflect.StructField) string {
	name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
	if name == "-" {
		return ""
	}

	return name
}

func setupRouter(queries store.Querier) *gin.Engine {
	baseURL := baseURLFromEnv()
	router := gin.New()
	router.TrustedPlatform = gin.PlatformCloudflare
	router.Use(
		corsMiddleware(),
		gin.Logger(),
		gin.Recovery(),
		sentrygin.New(sentrygin.Options{
			Repanic: true,
		}),
	)

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	registerLinkRoutes(router, queries, baseURL)
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	return router
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Origin") == frontendOrigin {
			c.Header("Access-Control-Allow-Origin", frontendOrigin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, Range")
			c.Header("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges")
			c.Header("Vary", "Origin")

			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}

		c.Next()
	}
}

func registerLinkRoutes(router *gin.Engine, queries store.Querier, baseURL string) {
	router.GET("/api/links", func(c *gin.Context) {
		requestedRange, hasRange, ok := parseLinksRange(c)
		if !ok {
			return
		}

		links, total, responseStart, err := listLinks(c.Request.Context(), queries, requestedRange, hasRange)
		if err != nil {
			respondInternalError(c)
			return
		}

		setLinksRangeHeaders(c, responseStart, int64(len(links)), total)

		response := make([]linkResponse, len(links))
		for i, link := range links {
			response[i] = linkToResponse(link, baseURL)
		}

		c.JSON(http.StatusOK, response)
	})

	router.GET("/api/link_visits", func(c *gin.Context) {
		requestedRange, hasRange, ok := parseLinksRange(c)
		if !ok {
			return
		}

		visits, total, responseStart, err := listLinkVisits(c.Request.Context(), queries, requestedRange, hasRange)
		if err != nil {
			respondInternalError(c)
			return
		}

		setRangeHeaders(c, linkVisitsRangeUnit, responseStart, int64(len(visits)), total)

		response := make([]linkVisitResponse, len(visits))
		for i, visit := range visits {
			response[i] = linkVisitToResponse(visit)
		}

		c.JSON(http.StatusOK, response)
	})

	router.POST("/api/links", func(c *gin.Context) {
		request, ok := bindCreateLinkPayload(c)
		if !ok {
			return
		}

		hasProvidedShortName := request.ShortName != ""
		for attempt := 0; attempt < maxShortNameAttempts; attempt++ {
			shortName := request.ShortName
			if shortName == "" {
				generated, err := generateShortName()
				if err != nil {
					respondInternalError(c)
					return
				}
				shortName = generated
			}

			link, err := queries.CreateLink(c.Request.Context(), store.CreateLinkParams{
				OriginalUrl: request.OriginalURL,
				ShortName:   shortName,
			})
			if err == nil {
				c.JSON(http.StatusCreated, linkToResponse(link, baseURL))
				return
			}

			if isUniqueViolation(err) {
				if hasProvidedShortName {
					respondShortNameInUse(c)
					return
				}

				continue
			}

			respondInternalError(c)
			return
		}

		respondInternalError(c)
	})

	router.GET("/api/links/:id", func(c *gin.Context) {
		id, ok := parseLinkID(c)
		if !ok {
			return
		}

		link, err := queries.GetLink(c.Request.Context(), id)
		if err != nil {
			respondStoreError(c, err)
			return
		}

		c.JSON(http.StatusOK, linkToResponse(link, baseURL))
	})

	router.PUT("/api/links/:id", func(c *gin.Context) {
		id, ok := parseLinkID(c)
		if !ok {
			return
		}

		request, ok := bindUpdateLinkPayload(c)
		if !ok {
			return
		}

		shortName := request.ShortName
		if shortName == "" {
			existing, err := queries.GetLink(c.Request.Context(), id)
			if err != nil {
				respondStoreError(c, err)
				return
			}

			shortName = existing.ShortName
		}

		link, err := queries.UpdateLink(c.Request.Context(), store.UpdateLinkParams{
			ID:          id,
			OriginalUrl: request.OriginalURL,
			ShortName:   shortName,
		})
		if err != nil {
			respondStoreError(c, err)
			return
		}

		c.JSON(http.StatusOK, linkToResponse(link, baseURL))
	})

	router.DELETE("/api/links/:id", func(c *gin.Context) {
		id, ok := parseLinkID(c)
		if !ok {
			return
		}

		rowsAffected, err := queries.DeleteLink(c.Request.Context(), id)
		if err != nil {
			respondInternalError(c)
			return
		}

		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
			return
		}

		c.Status(http.StatusNoContent)
	})

	router.GET("/r/:code", func(c *gin.Context) {
		link, err := queries.GetLinkByShortName(c.Request.Context(), c.Param("code"))
		if err != nil {
			respondStoreError(c, err)
			return
		}

		redirectStatus := http.StatusFound
		if _, err := queries.CreateLinkVisit(c.Request.Context(), store.CreateLinkVisitParams{
			LinkID:    link.ID,
			Ip:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Referer:   c.Request.Referer(),
			Status:    int32(redirectStatus),
		}); err != nil {
			respondInternalError(c)
			return
		}

		c.Redirect(redirectStatus, link.OriginalUrl)
	})
}

func listLinks(ctx context.Context, queries store.Querier, requestedRange linksRange, hasRange bool) ([]store.Link, int64, int64, error) {
	if !hasRange {
		links, err := queries.ListLinks(ctx)
		if err != nil {
			return nil, 0, 0, err
		}

		return links, int64(len(links)), 0, nil
	}

	total, err := queries.CountLinks(ctx)
	if err != nil {
		return nil, 0, 0, err
	}

	links, err := queries.ListLinksPage(ctx, store.ListLinksPageParams{
		PageOffset: int32(requestedRange.start),
		PageLimit:  int32(requestedRange.limit()),
	})
	if err != nil {
		return nil, 0, 0, err
	}

	return links, total, requestedRange.start, nil
}

func listLinkVisits(ctx context.Context, queries store.Querier, requestedRange linksRange, hasRange bool) ([]store.LinkVisit, int64, int64, error) {
	if !hasRange {
		visits, err := queries.ListLinkVisits(ctx)
		if err != nil {
			return nil, 0, 0, err
		}

		return visits, int64(len(visits)), 0, nil
	}

	total, err := queries.CountLinkVisits(ctx)
	if err != nil {
		return nil, 0, 0, err
	}

	visits, err := queries.ListLinkVisitsPage(ctx, store.ListLinkVisitsPageParams{
		PageOffset: int32(requestedRange.start),
		PageLimit:  int32(requestedRange.limit()),
	})
	if err != nil {
		return nil, 0, 0, err
	}

	return visits, total, requestedRange.start, nil
}

func parseLinksRange(c *gin.Context) (linksRange, bool, bool) {
	rawRange, hasRange := c.GetQuery("range")
	if !hasRange {
		rawRange = c.GetHeader("Range")
		hasRange = rawRange != ""
	}

	if !hasRange {
		return linksRange{}, false, true
	}

	requestedRange, err := parseLinksRangeValue(rawRange)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid range"})
		return linksRange{}, true, false
	}

	return requestedRange, true, true
}

func parseLinksRangeValue(rawRange string) (linksRange, error) {
	trimmed := strings.TrimSpace(rawRange)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return linksRange{}, errors.New("range must be wrapped in brackets")
	}

	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"), ",")
	if len(parts) != 2 {
		return linksRange{}, errors.New("range must include start and end")
	}

	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return linksRange{}, fmt.Errorf("parse range start: %w", err)
	}

	end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
	if err != nil {
		return linksRange{}, fmt.Errorf("parse range end: %w", err)
	}

	if start < 0 || end < start {
		return linksRange{}, errors.New("range boundaries must be non-negative and ordered")
	}

	if end-start+1 > maxLinksRangeLimit {
		return linksRange{}, errors.New("range is too large")
	}

	return linksRange{start: start, end: end}, nil
}

func (r linksRange) limit() int64 {
	return r.end - r.start + 1
}

func setLinksRangeHeaders(c *gin.Context, start, count, total int64) {
	setRangeHeaders(c, linksRangeUnit, start, count, total)
}

func linksContentRange(start, count, total int64) string {
	return contentRange(linksRangeUnit, start, count, total)
}

func setRangeHeaders(c *gin.Context, unit string, start, count, total int64) {
	c.Header("Accept-Ranges", unit)
	c.Header("Content-Range", contentRange(unit, start, count, total))
}

func contentRange(unit string, start, count, total int64) string {
	if count == 0 {
		return fmt.Sprintf("%s */%d", unit, total)
	}

	return fmt.Sprintf("%s %d-%d/%d", unit, start, start+count-1, total)
}

func bindCreateLinkPayload(c *gin.Context) (createLinkPayload, bool) {
	var request createLinkPayload
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		respondBindingError(c, err)
		return createLinkPayload{}, false
	}

	request.OriginalURL = strings.TrimSpace(request.OriginalURL)
	request.ShortName = strings.TrimSpace(request.ShortName)

	if err := validatePayload(request); err != nil {
		respondBindingError(c, err)
		return createLinkPayload{}, false
	}

	return request, true
}

func bindUpdateLinkPayload(c *gin.Context) (updateLinkPayload, bool) {
	var request updateLinkPayload
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		respondBindingError(c, err)
		return updateLinkPayload{}, false
	}

	request.OriginalURL = strings.TrimSpace(request.OriginalURL)
	request.ShortName = strings.TrimSpace(request.ShortName)

	if err := validatePayload(request); err != nil {
		respondBindingError(c, err)
		return updateLinkPayload{}, false
	}

	return request, true
}

func validatePayload(payload any) error {
	return binding.Validator.ValidateStruct(payload)
}

func parseLinkID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid link id"})
		return 0, false
	}

	return id, true
}

func linkToResponse(link store.Link, baseURL string) linkResponse {
	return linkResponse{
		ID:          link.ID,
		OriginalURL: link.OriginalUrl,
		ShortName:   link.ShortName,
		ShortURL:    shortURL(baseURL, link.ShortName),
	}
}

func linkVisitToResponse(visit store.LinkVisit) linkVisitResponse {
	response := linkVisitResponse{
		ID:        visit.ID,
		LinkID:    visit.LinkID,
		IP:        visit.Ip,
		UserAgent: visit.UserAgent,
		Status:    visit.Status,
	}

	if visit.CreatedAt.Valid {
		response.CreatedAt = visit.CreatedAt.Time
	}

	return response
}

func shortURL(baseURL, shortName string) string {
	return strings.TrimRight(baseURL, "/") + "/r/" + shortName
}

func generateShortName() (string, error) {
	bytes := make([]byte, generatedNameBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate short name: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func respondStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
	case isUniqueViolation(err):
		respondShortNameInUse(c)
	default:
		respondInternalError(c)
	}
}

func respondBindingError(c *gin.Context, err error) {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": validationErrorsByField(validationErrors)})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
}

func validationErrorsByField(validationErrors validator.ValidationErrors) map[string]string {
	errorsByField := make(map[string]string, len(validationErrors))
	for _, fieldError := range validationErrors {
		errorsByField[fieldError.Field()] = fieldError.Error()
	}

	return errorsByField
}

func respondShortNameInUse(c *gin.Context) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": gin.H{"short_name": "short name already in use"}})
}

func respondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func sentryClientOptionsFromEnv() sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn: os.Getenv("SENTRY_DSN"),
	}
}

func baseURLFromEnv() string {
	baseURL := strings.TrimSpace(os.Getenv("BASE_URL"))
	if baseURL == "" {
		return defaultBaseURL
	}

	return baseURL
}

func databaseURLFromEnv() string {
	return os.Getenv("DATABASE_URL")
}

func loadEnvFile(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load env file: %w", err)
	}

	return nil
}

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatal(err)
	}

	if err := sentry.Init(sentryClientOptionsFromEnv()); err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	pool, err := database.Connect(context.Background(), databaseURLFromEnv())
	if err != nil {
		log.Fatalf("database.Connect: %s", err)
	}
	defer pool.Close()

	queries := store.New(pool)
	router := setupRouter(queries)

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
