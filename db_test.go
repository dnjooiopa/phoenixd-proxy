package main

import (
	"testing"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
}

func TestInitDB(t *testing.T) {
	setupTestDB(t)

	// Verify table exists by inserting a row
	_, err := db.Exec("INSERT INTO endpoints (url) VALUES (?)", "https://example.com")
	if err != nil {
		t.Fatalf("expected endpoints table to exist: %v", err)
	}
}

func TestCreateEndpoint(t *testing.T) {
	setupTestDB(t)

	ep, err := CreateEndpoint(db, "https://example.com/hook")
	if err != nil {
		t.Fatalf("CreateEndpoint failed: %v", err)
	}
	if ep.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if ep.URL != "https://example.com/hook" {
		t.Errorf("expected URL 'https://example.com/hook', got '%s'", ep.URL)
	}
	if ep.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
}

func TestCreateEndpointDuplicateDB(t *testing.T) {
	setupTestDB(t)

	_, err := CreateEndpoint(db, "https://example.com/hook")
	if err != nil {
		t.Fatalf("first CreateEndpoint failed: %v", err)
	}

	_, err = CreateEndpoint(db, "https://example.com/hook")
	if err == nil {
		t.Fatal("expected error for duplicate URL, got nil")
	}
}

func TestGetAllEndpoints(t *testing.T) {
	setupTestDB(t)

	// Empty initially
	endpoints, err := GetAllEndpoints(db)
	if err != nil {
		t.Fatalf("GetAllEndpoints failed: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(endpoints))
	}

	// Add two
	CreateEndpoint(db, "https://a.com")
	CreateEndpoint(db, "https://b.com")

	endpoints, err = GetAllEndpoints(db)
	if err != nil {
		t.Fatalf("GetAllEndpoints failed: %v", err)
	}
	if len(endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestDeleteEndpointDB(t *testing.T) {
	setupTestDB(t)

	ep, _ := CreateEndpoint(db, "https://example.com/hook")

	err := DeleteEndpoint(db, ep.ID)
	if err != nil {
		t.Fatalf("DeleteEndpoint failed: %v", err)
	}

	endpoints, _ := GetAllEndpoints(db)
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints after delete, got %d", len(endpoints))
	}
}

func TestDeleteEndpointNotFoundDB(t *testing.T) {
	setupTestDB(t)

	err := DeleteEndpoint(db, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent ID, got nil")
	}
}
