package main

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleCreateInvoice(phoenixdURL, phoenixdPassword string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := c.Request.ParseForm(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse form"})
			return
		}

		description := c.PostForm("description")
		descriptionHash := c.PostForm("descriptionHash")
		if description == "" && descriptionHash == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "description or descriptionHash is required"})
			return
		}

		form := make(map[string]string)
		if description != "" {
			form["description"] = description
		}
		if descriptionHash != "" {
			form["descriptionHash"] = descriptionHash
		}
		for _, key := range []string{"amountSat", "expirySeconds", "externalId", "webhookUrl"} {
			if v := c.PostForm(key); v != "" {
				form[key] = v
			}
		}

		formValues := url.Values{}
		for k, v := range form {
			formValues.Set(k, v)
		}

		req, err := http.NewRequest(http.MethodPost, phoenixdURL+"/createinvoice", strings.NewReader(formValues.Encode()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
