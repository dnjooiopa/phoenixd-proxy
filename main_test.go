package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const testAPIKey = "test-secret-key"

func setupTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var err error
	db, err = InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return setupRouter(testAPIKey)
}

func TestListEndpointsUnauthorized(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/endpoints", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListEndpointsEmpty(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/endpoints", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var endpoints []Endpoint
	json.Unmarshal(w.Body.Bytes(), &endpoints)
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(endpoints))
	}
}

func TestCreateAndListEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	// Create
	body := bytes.NewBufferString(`{"url":"https://example.com/hook"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/endpoints", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var ep Endpoint
	json.Unmarshal(w.Body.Bytes(), &ep)
	if ep.URL != "https://example.com/hook" {
		t.Errorf("unexpected URL: %s", ep.URL)
	}

	// List
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/endpoints", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	var endpoints []Endpoint
	json.Unmarshal(w.Body.Bytes(), &endpoints)
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(endpoints))
	}
}

func TestCreateEndpointDuplicate(t *testing.T) {
	r := setupTestRouter(t)

	body := `{"url":"https://example.com/hook"}`
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/endpoints", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-KEY", testAPIKey)
		r.ServeHTTP(w, req)

		if i == 0 && w.Code != http.StatusCreated {
			t.Errorf("first create: expected 201, got %d", w.Code)
		}
		if i == 1 && w.Code != http.StatusConflict {
			t.Errorf("duplicate create: expected 409, got %d", w.Code)
		}
	}
}

func TestDeleteEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	// Create first
	body := bytes.NewBufferString(`{"url":"https://example.com/hook"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/endpoints", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	var ep Endpoint
	json.Unmarshal(w.Body.Bytes(), &ep)

	// Delete
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/endpoints/1", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestDeleteEndpointNotFound(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/endpoints/9999", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestWebhookSavesRequest(t *testing.T) {
	r := setupTestRouter(t)

	body := bytes.NewBufferString(`{"type":"payment_received","amountSat":100}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig456")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify it was saved
	requests, err := GetAllWebhookRequests(db, 100)
	if err != nil {
		t.Fatalf("GetAllWebhookRequests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 saved request, got %d", len(requests))
	}
	if requests[0].Body != `{"type":"payment_received","amountSat":100}` {
		t.Errorf("unexpected body: %s", requests[0].Body)
	}
	if requests[0].ContentType != "application/json" {
		t.Errorf("unexpected content_type: %s", requests[0].ContentType)
	}
	if requests[0].Signature != "sig456" {
		t.Errorf("unexpected signature: %s", requests[0].Signature)
	}
}

func TestListWebhookRequestsUnauthorized(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhook-requests", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListWebhookRequestsEmpty(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhook-requests", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var requests []WebhookRequest
	json.Unmarshal(w.Body.Bytes(), &requests)
	if len(requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(requests))
	}
}

func TestWebhookNoAuth(t *testing.T) {
	r := setupTestRouter(t)

	// Webhook should work without API key
	body := bytes.NewBufferString(`{"type":"payment_received","amountSat":1}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", resp["status"])
	}
}
