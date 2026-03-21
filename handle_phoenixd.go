package main

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func handlePhoenixdProxy(phoenixdURL, phoenixdPassword string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Param("path")

		req, err := http.NewRequest(c.Request.Method, phoenixdURL+path, c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
			return
		}
		req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
		req.SetBasicAuth("", phoenixdPassword)

		resp, err := httpClient.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach phoenixd"})
			return
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read phoenixd response"})
			return
		}

		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
	}
}
