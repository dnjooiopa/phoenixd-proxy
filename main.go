package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

const dbKey = "db"

func getDB(c *gin.Context) *DB {
	return c.MustGet(dbKey).(*DB)
}

func dbMiddleware(db *DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(dbKey, db)
		c.Next()
	}
}

func authRequired(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-KEY")
		if key == "" || key != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func setupRouter(db *DB, apiKey, phoenixdURL, phoenixdPassword string) *gin.Engine {
	r := gin.Default()
	r.Use(dbMiddleware(db))

	// Endpoint management (protected)
	ep := r.Group("/endpoints")
	ep.Use(authRequired(apiKey))
	{
		ep.GET("", handleListEndpoints)
		ep.POST("", handleCreateEndpoint)
		ep.DELETE("/:id", handleDeleteEndpoint)
	}

	// Webhook requests (protected)
	wr := r.Group("/webhook-requests")
	wr.Use(authRequired(apiKey))
	{
		wr.GET("", handleListWebhookRequests)
	}

	// Phoenixd proxy (protected)
	phoenixd := r.Group("/phoenixd/proxy")
	phoenixd.Use(authRequired(apiKey))
	{
		phoenixd.Any("/*path", handlePhoenixdProxy(phoenixdURL, phoenixdPassword))
	}

	// Webhook receiver (no auth)
	r.POST("/webhook", handleWebhook)

	return r
}

func main() {
	time.Local = time.UTC

	phoenixdURL := os.Getenv("PHOENIXD_URL")
	if phoenixdURL == "" {
		log.Fatal("PHOENIXD_URL environment variable is required")
	}

	phoenixdPassword := os.Getenv("PHOENIXD_PASSWORD")
	if phoenixdPassword == "" {
		log.Fatal("PHOENIXD_PASSWORD environment variable is required")
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable is required")
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	dbPath := "./data/proxy.db"
	if env == "production" {
		dbPath = "/app/data/proxy.db"
	}

	db, err := NewDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	addr := os.Getenv("ADDRESS")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := setupRouter(db, apiKey, phoenixdURL, phoenixdPassword)
	log.Printf("starting server on :%s", port)
	if err := r.Run(addr + ":" + port); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
