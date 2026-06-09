package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	repository "github.com/JackBraunYKT/go-project-278/internal/repository"
)

// RedirectToLink возвращает обработчик, который записывает посещение и перенаправляет на исходный URL.
func RedirectToLink(queries *repository.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		shortName := c.Param("code")
		if shortName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid short name"})
			return
		}

		link, err := queries.LinkByShortName(c.Request.Context(), shortName)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		_, err = queries.CreateLinkVisit(c.Request.Context(), repository.CreateLinkVisitParams{
			LinkID:    link.ID,
			Ip:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Status:    http.StatusMovedPermanently,
			Referer:   c.Request.Referer(),
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create link visit"})
			return
		}

		c.Redirect(http.StatusMovedPermanently, link.OriginalUrl)
	}
}
