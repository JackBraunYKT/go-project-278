package api

import (
	"net/http"

	repository "github.com/JackBraunYKT/go-project-278/internal/repository"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/logger"
	"github.com/gin-gonic/gin"
)

// SetupRouter инициализирует Gin-роутер с middleware и маршрутами приложения.
func SetupRouter(queries *repository.Queries) *gin.Engine {
	router := gin.New()

	if gin.Mode() != gin.TestMode {
		router.Use(logger.SetLogger())
		router.Use(sentrygin.New(sentrygin.Options{
			Repanic:         true,
			WaitForDelivery: true,
		}))
		router.Use(cors.New(cors.Config{
			AllowOrigins: []string{"http://localhost:5173"},
			AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
		}))
	}

	router.TrustedPlatform = gin.PlatformCloudflare

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, "pong")
	})

	router.GET("/api/links", GetLinks(queries))
	router.GET("/api/links/:id", GetLink(queries))
	router.POST("/api/links", CreateLink(queries))
	router.PUT("/api/links/:id", UpdateLink(queries))
	router.DELETE("/api/links/:id", DeleteLink(queries))
	router.GET("/r/:code", RedirectToLink(queries))
	router.GET("/api/link_visits", GetStatistics(queries))

	return router
}
