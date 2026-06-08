package main

import (
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
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

	return router
}

func sentryClientOptionsFromEnv() sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn: os.Getenv("SENTRY_DSN"),
	}
}

func main() {
	if err := sentry.Init(sentryClientOptionsFromEnv()); err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	router := setupRouter()

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
