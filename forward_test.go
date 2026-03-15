package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestForwardToAll(t *testing.T) {
	var mu sync.Mutex
	var receivedBody string
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(body)
		receivedHeaders = r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoints := []Endpoint{
		{ID: 1, URL: server.URL},
	}
	body := []byte(`{"type":"payment_received","amountSat":1}`)
	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Phoenix-Signature": "abc123",
	}

	ForwardToAll(endpoints, body, headers)

	mu.Lock()
	defer mu.Unlock()

	if receivedBody != `{"type":"payment_received","amountSat":1}` {
		t.Errorf("unexpected body: %s", receivedBody)
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected Content-Type: %s", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-Phoenix-Signature") != "abc123" {
		t.Errorf("unexpected X-Phoenix-Signature: %s", receivedHeaders.Get("X-Phoenix-Signature"))
	}
}

func TestForwardToAllMultiple(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoints := []Endpoint{
		{ID: 1, URL: server.URL + "/a"},
		{ID: 2, URL: server.URL + "/b"},
		{ID: 3, URL: server.URL + "/c"},
	}

	ForwardToAll(endpoints, []byte(`{}`), map[string]string{"Content-Type": "application/json"})

	mu.Lock()
	defer mu.Unlock()
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestForwardToAllUnreachable(t *testing.T) {
	// Should not panic when endpoint is unreachable
	endpoints := []Endpoint{
		{ID: 1, URL: "http://127.0.0.1:1"},
	}

	ForwardToAll(endpoints, []byte(`{}`), map[string]string{"Content-Type": "application/json"})
}
