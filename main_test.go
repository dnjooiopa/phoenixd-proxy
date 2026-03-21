package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testAPIKey = "test-secret-key"

func setupTestRouter(t *testing.T) *gin.Engine {
	return setupTestRouterWithPhoenixd(t, "", "")
}

func setupTestRouterWithPhoenixd(t *testing.T, phoenixdURL, phoenixdPassword string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var err error
	db, err = InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return setupRouter(testAPIKey, phoenixdURL, phoenixdPassword)
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

	var resp struct {
		Data []Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(resp.Data))
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

	var createResp struct {
		Data Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	if createResp.Data.URL != "https://example.com/hook" {
		t.Errorf("unexpected URL: %s", createResp.Data.URL)
	}

	// List
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/endpoints", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	var listResp struct {
		Data []Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(listResp.Data))
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

	var resp struct {
		Data []WebhookRequest `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 requests, got %d", len(resp.Data))
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

	var webhookResp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &webhookResp)
	if webhookResp.Data.Status != "ok" {
		t.Errorf("expected status 'ok', got '%v'", webhookResp.Data.Status)
	}
}

func TestPhoenixdProxyUnauthorized(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/phoenixd/proxy/createinvoice", strings.NewReader("description=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPhoenixdProxyPostCreateInvoice(t *testing.T) {
	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/createinvoice" {
			t.Errorf("expected path '/createinvoice', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		_, password, ok := r.BasicAuth()
		if !ok || password != "test-phoenixd-pass" {
			t.Errorf("expected basic auth with password 'test-phoenixd-pass'")
		}

		r.ParseForm()
		if r.PostForm.Get("description") != "my first invoice" {
			t.Errorf("expected description 'my first invoice', got '%s'", r.PostForm.Get("description"))
		}
		if r.PostForm.Get("amountSat") != "100" {
			t.Errorf("expected amountSat '100', got '%s'", r.PostForm.Get("amountSat"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"amountSat":100,"paymentHash":"abc123","serialized":"lntb1u1..."}`))
	}))
	defer phoenixd.Close()

	r := setupTestRouterWithPhoenixd(t, phoenixd.URL, "test-phoenixd-pass")

	w := httptest.NewRecorder()
	body := "description=my+first+invoice&amountSat=100&externalId=foobar"
	req, _ := http.NewRequest("POST", "/phoenixd/proxy/createinvoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["amountSat"] != float64(100) {
		t.Errorf("expected amountSat 100, got %v", resp["amountSat"])
	}
	if resp["paymentHash"] != "abc123" {
		t.Errorf("expected paymentHash 'abc123', got %v", resp["paymentHash"])
	}
}

func TestPhoenixdProxyGetNodeInfo(t *testing.T) {
	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getinfo" {
			t.Errorf("expected path '/getinfo', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"nodeId":"02abc","channels":[]}`))
	}))
	defer phoenixd.Close()

	r := setupTestRouterWithPhoenixd(t, phoenixd.URL, "test-pass")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/phoenixd/proxy/getinfo", nil)
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["nodeId"] != "02abc" {
		t.Errorf("expected nodeId '02abc', got %v", resp["nodeId"])
	}
}

func TestPhoenixdProxyPhoenixdError(t *testing.T) {
	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid amount"}`))
	}))
	defer phoenixd.Close()

	r := setupTestRouterWithPhoenixd(t, phoenixd.URL, "test-phoenixd-pass")

	w := httptest.NewRecorder()
	body := "description=test&amountSat=-1"
	req, _ := http.NewRequest("POST", "/phoenixd/proxy/createinvoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-API-KEY", testAPIKey)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
