package main

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Endpoint struct {
	ID        int64      `json:"id"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type WebhookRequest struct {
	ID          int64     `json:"id"`
	Body        string    `json:"body"`
	ContentType string    `json:"content_type"`
	Signature   string    `json:"signature"`
	CreatedAt   time.Time `json:"created_at"`
}

var ErrNotFound = errors.New("not found")

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS endpoints (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			url        TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP
		)
	`)
	if err != nil {
		return nil, err
	}

	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_requests (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			body         TEXT NOT NULL,
			content_type TEXT NOT NULL DEFAULT '',
			signature    TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, err
	}

	_, err = conn.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_endpoints_url_active
		ON endpoints (url) WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) GetAllEndpoints() ([]Endpoint, error) {
	rows, err := d.conn.Query("SELECT id, url, created_at FROM endpoints WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		var ep Endpoint
		if err := rows.Scan(&ep.ID, &ep.URL, &ep.CreatedAt); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}

	if endpoints == nil {
		endpoints = []Endpoint{}
	}

	return endpoints, rows.Err()
}

func (d *DB) CreateEndpoint(url string) (Endpoint, error) {
	var ep Endpoint
	err := d.conn.QueryRow(
		"INSERT INTO endpoints (url) VALUES (?) RETURNING id, url, created_at",
		url,
	).Scan(&ep.ID, &ep.URL, &ep.CreatedAt)
	if err != nil {
		return Endpoint{}, err
	}
	return ep, nil
}

func (d *DB) CreateWebhookRequest(body, contentType, signature string) (WebhookRequest, error) {
	var wr WebhookRequest
	err := d.conn.QueryRow(
		"INSERT INTO webhook_requests (body, content_type, signature) VALUES (?, ?, ?) RETURNING id, body, content_type, signature, created_at",
		body, contentType, signature,
	).Scan(&wr.ID, &wr.Body, &wr.ContentType, &wr.Signature, &wr.CreatedAt)
	if err != nil {
		return WebhookRequest{}, err
	}
	return wr, nil
}

func (d *DB) GetAllWebhookRequests(limit int) ([]WebhookRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := d.conn.Query("SELECT id, body, content_type, signature, created_at FROM webhook_requests ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []WebhookRequest
	for rows.Next() {
		var wr WebhookRequest
		if err := rows.Scan(&wr.ID, &wr.Body, &wr.ContentType, &wr.Signature, &wr.CreatedAt); err != nil {
			return nil, err
		}
		requests = append(requests, wr)
	}

	if requests == nil {
		requests = []WebhookRequest{}
	}

	return requests, rows.Err()
}

func (d *DB) DeleteEndpoint(id int64) error {
	result, err := d.conn.Exec(
		"UPDATE endpoints SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
