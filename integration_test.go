package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// helper to make authenticated requests
func authReq(method, path string, body io.Reader) *http.Request {
	req, _ := http.NewRequest(method, path, body)
	req.Header.Set("X-API-KEY", testAPIKey)
	return req
}

func jsonAuthReq(method, path, jsonBody string) *http.Request {
	req := authReq(method, path, strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Endpoint handler edge cases ---

func TestCreateEndpointMissingBody(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", ""))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEndpointEmptyURL(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", `{"url":""}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEndpointWhitespaceURL(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"   "}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEndpointInvalidJSON(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", `{invalid`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEndpointMissingURLField(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", `{"name":"test"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEndpointURLTrimmed(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"  https://example.com/hook  "}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp struct {
		Data Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.URL != "https://example.com/hook" {
		t.Errorf("expected trimmed URL, got '%s'", resp.Data.URL)
	}
}

func TestDeleteEndpointInvalidID(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := serve(r, authReq("DELETE", "/endpoints/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAuthWrongAPIKey(t *testing.T) {
	r, _ := setupTestRouter(t)

	req, _ := http.NewRequest("GET", "/endpoints", nil)
	req.Header.Set("X-API-KEY", "wrong-key")
	w := serve(r, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Webhook request listing edge cases ---

func TestListWebhookRequestsWithLimit(t *testing.T) {
	r, db := setupTestRouter(t)

	// Insert 5 webhook requests
	for range 5 {
		db.CreateWebhookRequest("body", "text/plain", "sig")
	}

	w := serve(r, authReq("GET", "/webhook-requests?limit=3", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []WebhookRequest `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 3 {
		t.Errorf("expected 3 requests with limit=3, got %d", len(resp.Data))
	}
}

func TestListWebhookRequestsInvalidLimit(t *testing.T) {
	r, db := setupTestRouter(t)

	db.CreateWebhookRequest("body", "text/plain", "sig")

	// Invalid limit should fall back to default (100)
	w := serve(r, authReq("GET", "/webhook-requests?limit=abc", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []WebhookRequest `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 request, got %d", len(resp.Data))
	}
}

// --- Webhook forwarding integration ---

func TestWebhookForwardsToRegisteredEndpoints(t *testing.T) {
	var mu sync.Mutex
	var receivedBodies []string

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	r, _ := setupTestRouter(t)

	// Register an endpoint
	serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`"}`))

	// Send a webhook
	payload := `{"type":"payment_received","amountSat":500}`
	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "testsig")
	w := serve(r, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			Status      string `json:"status"`
			ForwardedTo int    `json:"forwarded_to"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.ForwardedTo != 1 {
		t.Errorf("expected forwarded_to=1, got %d", resp.Data.ForwardedTo)
	}

	// Wait briefly for async forwarding goroutine
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(receivedBodies) != 1 {
		t.Fatalf("expected 1 forwarded request, got %d", len(receivedBodies))
	}
	if receivedBodies[0] != payload {
		t.Errorf("expected forwarded body '%s', got '%s'", payload, receivedBodies[0])
	}
}

func TestWebhookForwardsToMultipleEndpoints(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	r, _ := setupTestRouter(t)

	// Register two endpoints
	serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`/a"}`))
	serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`/b"}`))

	// Send a webhook
	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig")
	w := serve(r, req)

	var resp struct {
		Data struct {
			ForwardedTo int `json:"forwarded_to"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.ForwardedTo != 2 {
		t.Errorf("expected forwarded_to=2, got %d", resp.Data.ForwardedTo)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Errorf("expected 2 forwarded requests, got %d", callCount)
	}
}

func TestWebhookDoesNotForwardAfterEndpointDeleted(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	r, _ := setupTestRouter(t)

	// Register and then delete endpoint
	serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`"}`))
	serve(r, authReq("DELETE", "/endpoints/1", nil))

	// Send a webhook
	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig")
	w := serve(r, req)

	var resp struct {
		Data struct {
			ForwardedTo int `json:"forwarded_to"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.ForwardedTo != 0 {
		t.Errorf("expected forwarded_to=0 after deletion, got %d", resp.Data.ForwardedTo)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if callCount != 0 {
		t.Errorf("expected 0 forwarded requests after deletion, got %d", callCount)
	}
}

func TestWebhookStillSavedEvenWithNoEndpoints(t *testing.T) {
	r, db := setupTestRouter(t)

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig")
	w := serve(r, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	requests, err := db.GetAllWebhookRequests(100)
	if err != nil {
		t.Fatalf("GetAllWebhookRequests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Errorf("expected webhook to be saved even with no endpoints, got %d", len(requests))
	}
}

func TestWebhookForwardsSignatureHeader(t *testing.T) {
	var mu sync.Mutex
	var receivedSig string

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedSig = r.Header.Get("X-Phoenix-Signature")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	r, _ := setupTestRouter(t)

	serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`"}`))

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "hmac-sig-abc")
	serve(r, req)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedSig != "hmac-sig-abc" {
		t.Errorf("expected forwarded signature 'hmac-sig-abc', got '%s'", receivedSig)
	}
}

// --- Phoenixd proxy edge cases ---

func TestPhoenixdProxyUnreachable(t *testing.T) {
	r, _ := setupTestRouterWithPhoenixd(t, "http://127.0.0.1:1", "pass")

	w := serve(r, authReq("GET", "/phoenixd/proxy/getinfo", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "failed to reach phoenixd" {
		t.Errorf("unexpected error: %s", resp["error"])
	}
}

func TestPhoenixdProxyForwardsContentType(t *testing.T) {
	var receivedContentType string

	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer phoenixd.Close()

	r, _ := setupTestRouterWithPhoenixd(t, phoenixd.URL, "pass")

	req := authReq("POST", "/phoenixd/proxy/createinvoice", strings.NewReader("desc=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serve(r, req)

	if receivedContentType != "application/x-www-form-urlencoded" {
		t.Errorf("expected form content type, got '%s'", receivedContentType)
	}
}

func TestPhoenixdProxyPreservesResponseContentType(t *testing.T) {
	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text response"))
	}))
	defer phoenixd.Close()

	r, _ := setupTestRouterWithPhoenixd(t, phoenixd.URL, "pass")

	w := serve(r, authReq("GET", "/phoenixd/proxy/something", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected content-type 'text/plain', got '%s'", ct)
	}
	if w.Body.String() != "plain text response" {
		t.Errorf("expected 'plain text response', got '%s'", w.Body.String())
	}
}

func TestPhoenixdProxyDELETEMethod(t *testing.T) {
	var receivedMethod string

	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted":true}`))
	}))
	defer phoenixd.Close()

	r, _ := setupTestRouterWithPhoenixd(t, phoenixd.URL, "pass")

	w := serve(r, authReq("DELETE", "/phoenixd/proxy/some-resource", nil))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if receivedMethod != http.MethodDelete {
		t.Errorf("expected DELETE method, got %s", receivedMethod)
	}
}

func TestPhoenixdProxyPUTMethod(t *testing.T) {
	var receivedMethod string
	var receivedBody string

	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"updated":true}`))
	}))
	defer phoenixd.Close()

	r, _ := setupTestRouterWithPhoenixd(t, phoenixd.URL, "pass")

	w := serve(r, jsonAuthReq("PUT", "/phoenixd/proxy/resource", `{"key":"value"}`))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if receivedMethod != http.MethodPut {
		t.Errorf("expected PUT method, got %s", receivedMethod)
	}
	if receivedBody != `{"key":"value"}` {
		t.Errorf("expected body forwarded, got '%s'", receivedBody)
	}
}

func TestPhoenixdProxyNestedPath(t *testing.T) {
	var receivedPath string

	phoenixd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer phoenixd.Close()

	r, _ := setupTestRouterWithPhoenixd(t, phoenixd.URL, "pass")

	serve(r, authReq("GET", "/phoenixd/proxy/v1/payments/list", nil))
	if receivedPath != "/v1/payments/list" {
		t.Errorf("expected path '/v1/payments/list', got '%s'", receivedPath)
	}
}

// --- Full end-to-end flow ---

func TestEndToEndWebhookFlow(t *testing.T) {
	var mu sync.Mutex
	var forwardedPayloads []string

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		forwardedPayloads = append(forwardedPayloads, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	r, db := setupTestRouter(t)

	// Step 1: No endpoints initially
	w := serve(r, authReq("GET", "/endpoints", nil))
	var listResp struct {
		Data []Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 0 {
		t.Fatalf("expected 0 endpoints initially")
	}

	// Step 2: Register two endpoints
	w = serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`/a"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for first endpoint, got %d", w.Code)
	}
	w = serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"`+target.URL+`/b"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for second endpoint, got %d", w.Code)
	}

	// Step 3: Send a webhook
	payload := `{"type":"payment_received","amountSat":1000}`
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "e2e-sig")
	w = serve(r, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	time.Sleep(150 * time.Millisecond)

	// Step 4: Verify forwarding happened to both endpoints
	mu.Lock()
	if len(forwardedPayloads) != 2 {
		t.Errorf("expected 2 forwarded payloads, got %d", len(forwardedPayloads))
	}
	for i, p := range forwardedPayloads {
		if p != payload {
			t.Errorf("forwarded payload %d mismatch: %s", i, p)
		}
	}
	mu.Unlock()

	// Step 5: Verify webhook was saved
	requests, _ := db.GetAllWebhookRequests(100)
	if len(requests) != 1 {
		t.Fatalf("expected 1 saved webhook request, got %d", len(requests))
	}
	if requests[0].Body != payload {
		t.Errorf("saved body mismatch")
	}
	if requests[0].Signature != "e2e-sig" {
		t.Errorf("saved signature mismatch")
	}

	// Step 6: Verify via webhook-requests API
	w = serve(r, authReq("GET", "/webhook-requests", nil))
	var wrResp struct {
		Data []WebhookRequest `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &wrResp)
	if len(wrResp.Data) != 1 {
		t.Errorf("expected 1 webhook request from API, got %d", len(wrResp.Data))
	}

	// Step 7: Delete one endpoint and send another webhook
	serve(r, authReq("DELETE", "/endpoints/1", nil))

	forwardedPayloads = nil
	req, _ = http.NewRequest("POST", "/webhook", bytes.NewBufferString(`{"event":"second"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phoenix-Signature", "sig2")
	serve(r, req)

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if len(forwardedPayloads) != 1 {
		t.Errorf("expected 1 forwarded payload after deletion, got %d", len(forwardedPayloads))
	}
	mu.Unlock()

	// Step 8: Verify both webhooks are saved
	requests, _ = db.GetAllWebhookRequests(100)
	if len(requests) != 2 {
		t.Errorf("expected 2 total saved webhook requests, got %d", len(requests))
	}
}

func TestEndToEndEndpointLifecycle(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	w := serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"https://example.com/hook"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var createResp struct {
		Data Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp.Data.ID

	// Duplicate fails
	w = serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"https://example.com/hook"}`))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}

	// Delete
	w = serve(r, authReq("DELETE", "/endpoints/"+strings.TrimSpace(json.Number(string(rune('0'+id))).String()), nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Re-create same URL after soft delete
	w = serve(r, jsonAuthReq("POST", "/endpoints", `{"url":"https://example.com/hook"}`))
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 after soft delete re-create, got %d", w.Code)
	}

	// List shows only one active
	w = serve(r, authReq("GET", "/endpoints", nil))
	var listResp struct {
		Data []Endpoint `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Errorf("expected 1 active endpoint, got %d", len(listResp.Data))
	}
}
