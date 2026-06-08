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
	generatedNameBytes   = 6
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

func setupRouter(queries store.Querier) *gin.Engine {
	baseURL := baseURLFromEnv()
	router := gin.New()
	router.Use(
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

func registerLinkRoutes(router *gin.Engine, queries store.Querier, baseURL string) {
	router.GET("/api/links", func(c *gin.Context) {
		links, err := queries.ListLinks(c.Request.Context())
		if err != nil {
			respondInternalError(c)
			return
		}

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
