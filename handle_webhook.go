package main

import (
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func handleListWebhookRequests(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	requests, err := GetAllWebhookRequests(db, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": requests})
}

func handleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	contentType := c.GetHeader("Content-Type")
	signature := c.GetHeader("X-Phoenix-Signature")

	headers := map[string]string{
		"Content-Type":        contentType,
		"X-Phoenix-Signature": signature,
	}

	if _, err := CreateWebhookRequest(db, string(body), contentType, signature); err != nil {
		log.Printf("failed to save webhook request: %v", err)
	}

	endpoints, err := GetAllEndpoints(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(endpoints) > 0 {
		go ForwardToAll(endpoints, body, headers)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"status":       "ok",
		"forwarded_to": len(endpoints),
	}})
}
