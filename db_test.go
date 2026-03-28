package main

import (
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitDB(t *testing.T) {
	db := setupTestDB(t)

	// Verify table exists by inserting a row
	_, err := db.conn.Exec("INSERT INTO endpoints (url) VALUES (?)", "https://example.com")
	if err != nil {
		t.Fatalf("expected endpoints table to exist: %v", err)
	}
}

func TestCreateEndpoint(t *testing.T) {
	db := setupTestDB(t)

	ep, err := db.CreateEndpoint("https://example.com/hook")
	if err != nil {
		t.Fatalf("CreateEndpoint failed: %v", err)
	}
	if ep.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if ep.URL != "https://example.com/hook" {
		t.Errorf("expected URL 'https://example.com/hook', got '%s'", ep.URL)
	}
	if ep.CreatedAt.IsZero() {
		t.Error("expected non-empty CreatedAt")
	}
}

func TestCreateEndpointDuplicateDB(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.CreateEndpoint("https://example.com/hook")
	if err != nil {
		t.Fatalf("first CreateEndpoint failed: %v", err)
	}

	_, err = db.CreateEndpoint("https://example.com/hook")
	if err == nil {
		t.Fatal("expected error for duplicate URL, got nil")
	}
}

func TestGetAllEndpoints(t *testing.T) {
	db := setupTestDB(t)

	// Empty initially
	endpoints, err := db.GetAllEndpoints()
	if err != nil {
		t.Fatalf("GetAllEndpoints failed: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(endpoints))
	}

	// Add two
	db.CreateEndpoint("https://a.com")
	db.CreateEndpoint("https://b.com")

	endpoints, err = db.GetAllEndpoints()
	if err != nil {
		t.Fatalf("GetAllEndpoints failed: %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestDeleteEndpointDB(t *testing.T) {
	db := setupTestDB(t)

	ep, _ := db.CreateEndpoint("https://example.com/hook")

	err := db.DeleteEndpoint(ep.ID)
	if err != nil {
		t.Fatalf("DeleteEndpoint failed: %v", err)
	}

	endpoints, _ := db.GetAllEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints after delete, got %d", len(endpoints))
	}
}

func TestDeleteEndpointNotFoundDB(t *testing.T) {
	db := setupTestDB(t)

	err := db.DeleteEndpoint(9999)
	if err == nil {
		t.Fatal("expected error for non-existent ID, got nil")
	}
}

func TestSoftDeleteKeepsRecord(t *testing.T) {
	db := setupTestDB(t)

	ep, _ := db.CreateEndpoint("https://example.com/hook")
	db.DeleteEndpoint(ep.ID)

	// Record should still exist in DB
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM endpoints WHERE id = ?", ep.ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected soft-deleted record to still exist, got count %d", count)
	}

	// But not returned by GetAllEndpoints
	endpoints, _ := db.GetAllEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("expected 0 active endpoints, got %d", len(endpoints))
	}
}

func TestSoftDeleteAllowsReuse(t *testing.T) {
	db := setupTestDB(t)

	ep, _ := db.CreateEndpoint("https://example.com/hook")
	db.DeleteEndpoint(ep.ID)

	// Should be able to re-create with the same URL
	ep2, err := db.CreateEndpoint("https://example.com/hook")
	if err != nil {
		t.Fatalf("expected re-create after soft delete to succeed: %v", err)
	}
	if ep2.URL != "https://example.com/hook" {
		t.Errorf("unexpected URL: %s", ep2.URL)
	}
}

func TestCreateWebhookRequest(t *testing.T) {
	db := setupTestDB(t)

	wr, err := db.CreateWebhookRequest(`{"type":"payment"}`, "application/json", "sig123")
	if err != nil {
		t.Fatalf("CreateWebhookRequest failed: %v", err)
	}
	if wr.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if wr.Body != `{"type":"payment"}` {
		t.Errorf("unexpected body: %s", wr.Body)
	}
	if wr.ContentType != "application/json" {
		t.Errorf("unexpected content_type: %s", wr.ContentType)
	}
	if wr.Signature != "sig123" {
		t.Errorf("unexpected signature: %s", wr.Signature)
	}
	if wr.CreatedAt.IsZero() {
		t.Error("expected non-empty CreatedAt")
	}
}

func TestGetAllWebhookRequests(t *testing.T) {
	db := setupTestDB(t)

	// Empty initially
	requests, err := db.GetAllWebhookRequests(100)
	if err != nil {
		t.Fatalf("GetAllWebhookRequests failed: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected 0 requests, got %d", len(requests))
	}

	// Add two
	db.CreateWebhookRequest("body1", "text/plain", "sig1")
	db.CreateWebhookRequest("body2", "text/plain", "sig2")

	requests, err = db.GetAllWebhookRequests(100)
	if err != nil {
		t.Fatalf("GetAllWebhookRequests failed: %v", err)
	}
	if len(requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(requests))
	}

	// Should be ordered by id DESC (newest first)
	if requests[0].Body != "body2" {
		t.Errorf("expected newest first, got %s", requests[0].Body)
	}
}

func TestDeleteAlreadyDeletedEndpoint(t *testing.T) {
	db := setupTestDB(t)

	ep, _ := db.CreateEndpoint("https://example.com/hook")
	db.DeleteEndpoint(ep.ID)

	// Deleting again should return not found
	err := db.DeleteEndpoint(ep.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for already-deleted endpoint, got %v", err)
	}
}
