package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var db *sql.DB

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

func setupRouter(apiKey string) *gin.Engine {
	r := gin.Default()

	// Endpoint management (protected)
	ep := r.Group("/endpoints")
	ep.Use(authRequired(apiKey))
	{
		ep.GET("", handleListEndpoints)
		ep.POST("", handleCreateEndpoint)
		ep.DELETE("/:id", handleDeleteEndpoint)
	}

	// Webhook receiver (no auth)
	r.POST("/webhook", handleWebhook)

	return r
}

func handleListEndpoints(c *gin.Context) {
	endpoints, err := GetAllEndpoints(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, endpoints)
}

func handleCreateEndpoint(c *gin.Context) {
	var input struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	ep, err := CreateEndpoint(db, input.URL)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			c.JSON(http.StatusConflict, gin.H{"error": "endpoint already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, ep)
}

func handleDeleteEndpoint(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := DeleteEndpoint(db, id); err != nil {
		if err == ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func handleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	headers := map[string]string{
		"Content-Type":        c.GetHeader("Content-Type"),
		"X-Phoenix-Signature": c.GetHeader("X-Phoenix-Signature"),
	}

	endpoints, err := GetAllEndpoints(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(endpoints) > 0 {
		go ForwardToAll(endpoints, body, headers)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"forwarded_to": len(endpoints),
	})
}

func main() {
	time.Local = time.UTC

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

	var err error
	db, err = InitDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	addr := os.Getenv("ADDRESS")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := setupRouter(apiKey)
	log.Printf("starting server on :%s", port)
	if err := r.Run(addr + ":" + port); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
