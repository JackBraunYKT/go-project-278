package api

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	config "github.com/JackBraunYKT/go-project-278/internal/config"
	repository "github.com/JackBraunYKT/go-project-278/internal/repository"
	service "github.com/JackBraunYKT/go-project-278/internal/service"
)

type LinkResponse struct {
	ID          int64  `json:"id"`
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
	ShortURL    string `json:"short_url"`
}

type createLinkPayload struct {
	OriginalURL string `json:"original_url" binding:"required,url"`
	ShortName   string `json:"short_name" binding:"omitempty,min=3,max=32"`
}

type updateLinkPayload struct {
	OriginalURL string `json:"original_url" binding:"omitempty,url"`
	ShortName   string `json:"short_name" binding:"omitempty,min=3,max=32"`
}

type validationErrorResponse struct {
	Errors map[string]string `json:"errors"`
}

func init() {
	if validatorEngine, ok := binding.Validator.Engine().(*validator.Validate); ok {
		validatorEngine.RegisterTagNameFunc(func(field reflect.StructField) string {
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				return field.Name
			}
			return name
		})
	}
}

func respondValidationErrors(c *gin.Context, err error) {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		messages := make(map[string]string, len(validationErrors))
		for _, validationErr := range validationErrors {
			messages[validationErr.Field()] = validationErr.Error()
		}
		c.JSON(http.StatusUnprocessableEntity, validationErrorResponse{Errors: messages})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
}

func respondShortNameInUse(c *gin.Context) {
	c.JSON(http.StatusUnprocessableEntity, validationErrorResponse{
		Errors: map[string]string{
			"short_name": "short name already in use",
		},
	})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func buildShortURL(shortName string) string {
	appURL := config.GetEnv("APP_URL", "https://short.io")
	return fmt.Sprintf("%s/r/%s", appURL, shortName)
}

// GetLinks возвращает обработчик списка ссылок с опциональной пагинацией по range.
func GetLinks(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var links []repository.Link
		var err error
		var requestedRange *rangeBounds
		paginationRange := c.Query("range")

		if paginationRange != "" {
			pagination, bounds, err := getLimitAndOffsetFromQuery(paginationRange)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			requestedRange = &bounds

			links, err = queries.ListLinks(c.Request.Context(), repository.ListLinksParams{
				Limit:  pagination.Limit,
				Offset: pagination.Offset,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			links, err = queries.ListAllLinks(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		total, err := queries.CountLinks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		contentRange := buildContentRange("links", requestedRange, len(links), total)
		c.Header("Content-Range", contentRange)

		responseLinks := make([]LinkResponse, 0, len(links))
		for _, link := range links {
			responseLinks = append(responseLinks, toLinkResponse(link))
		}

		c.JSON(http.StatusOK, responseLinks)
	}
}

// GetLink возвращает обработчик получения одной ссылки по ID.
func GetLink(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		if idStr == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "wrong id format"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		link, err := queries.GetLink(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, toLinkResponse(link))
	}
}

// CreateLink возвращает обработчик создания ссылки и генерирует короткое имя при необходимости.
func CreateLink(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input createLinkPayload
		if err := c.ShouldBindJSON(&input); err != nil {
			respondValidationErrors(c, err)
			return
		}

		shortName := input.ShortName
		if shortName == "" {
			randomString, err := service.GenerateShortName(5)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate short name"})
				return
			}
			shortName = randomString
		}

		shortURL := buildShortURL(shortName)
		created, err := queries.CreateLink(c.Request.Context(), repository.CreateLinkParams{
			OriginalUrl: input.OriginalURL,
			ShortName:   shortName,
			ShortUrl:    shortURL,
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondShortNameInUse(c)
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create link"})
			return
		}

		c.JSON(http.StatusCreated, toLinkResponse(created))
	}
}

// UpdateLink возвращает обработчик обновления существующей ссылки по ID.
func UpdateLink(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		if idStr == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "wrong id format"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		var input updateLinkPayload
		if err := c.ShouldBindJSON(&input); err != nil {
			respondValidationErrors(c, err)
			return
		}

		current, err := queries.GetLink(c.Request.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		originalURL := input.OriginalURL
		shortName := input.ShortName

		if shortName == "" {
			shortName = current.ShortName
		}

		shortURL := buildShortURL(shortName)

		if originalURL == "" {
			originalURL = current.OriginalUrl
		}

		rowsAffected, err := queries.UpdateLink(c.Request.Context(), repository.UpdateLinkParams{
			ID:          id,
			OriginalUrl: originalURL,
			ShortName:   shortName,
			ShortUrl:    shortURL,
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondShortNameInUse(c)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
			return
		}

		updated, err := queries.GetLink(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, toLinkResponse(updated))
	}
}

// DeleteLink возвращает обработчик удаления ссылки по ID.
func DeleteLink(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		if idStr == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "wrong id format"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		rowsAffected, err := queries.DeleteLink(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
			return
		}
		c.JSON(http.StatusNoContent, gin.H{})
	}
}

func toLinkResponse(l repository.Link) LinkResponse {
	return LinkResponse{
		ID:          l.ID,
		OriginalURL: l.OriginalUrl,
		ShortName:   l.ShortName,
		ShortURL:    l.ShortUrl,
	}
}
