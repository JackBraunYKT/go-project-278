package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JackBraunYKT/go-project-278/internal/database"
	"github.com/JackBraunYKT/go-project-278/internal/store"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/joho/godotenv"
)

const (
	defaultBaseURL       = "https://short.io"
	frontendOrigin       = "http://localhost:5173"
	generatedNameBytes   = 6
	linksRangeUnit       = "links"
	maxLinksRangeLimit   = int64(1<<31 - 1)
	maxShortNameAttempts = 5
)

type linkRequest struct {
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
}

type linkResponse struct {
	ID          int64  `json:"id"`
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
	ShortURL    string `json:"short_url"`
}

type linksRange struct {
	start int64
	end   int64
}

func setupRouter(queries store.Querier) *gin.Engine {
	baseURL := baseURLFromEnv()
	router := gin.New()
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

	router.POST("/api/links", func(c *gin.Context) {
		request, ok := bindLinkRequest(c, false)
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
					c.JSON(http.StatusConflict, gin.H{"error": "short_name already exists"})
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

		request, ok := bindLinkRequest(c, true)
		if !ok {
			return
		}

		link, err := queries.UpdateLink(c.Request.Context(), store.UpdateLinkParams{
			ID:          id,
			OriginalUrl: request.OriginalURL,
			ShortName:   request.ShortName,
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

func parseLinksRange(c *gin.Context) (linksRange, bool, bool) {
	rawRange, hasRange := c.GetQuery("range")
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
	c.Header("Accept-Ranges", linksRangeUnit)
	c.Header("Content-Range", linksContentRange(start, count, total))
}

func linksContentRange(start, count, total int64) string {
	if count == 0 {
		return fmt.Sprintf("%s */%d", linksRangeUnit, total)
	}

	return fmt.Sprintf("%s %d-%d/%d", linksRangeUnit, start, start+count-1, total)
}

func bindLinkRequest(c *gin.Context, requireShortName bool) (linkRequest, bool) {
	var request linkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return linkRequest{}, false
	}

	request.OriginalURL = strings.TrimSpace(request.OriginalURL)
	request.ShortName = strings.TrimSpace(request.ShortName)

	if request.OriginalURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "original_url is required"})
		return linkRequest{}, false
	}

	if requireShortName && request.ShortName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "short_name is required"})
		return linkRequest{}, false
	}

	return request, true
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
		c.JSON(http.StatusConflict, gin.H{"error": "short_name already exists"})
	default:
		respondInternalError(c)
	}
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
