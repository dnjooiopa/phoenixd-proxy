package main

import (
	"database/sql"
	"errors"

	_ "github.com/mattn/go-sqlite3"
)

type Endpoint struct {
	ID        int64  `json:"id"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

var ErrNotFound = errors.New("not found")

func InitDB(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)

	_, err = database.Exec(`
		CREATE TABLE IF NOT EXISTS endpoints (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			url        TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, err
	}

	return database, nil
}

func GetAllEndpoints(database *sql.DB) ([]Endpoint, error) {
	rows, err := database.Query("SELECT id, url, created_at FROM endpoints ORDER BY id")
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

func CreateEndpoint(database *sql.DB, url string) (Endpoint, error) {
	var ep Endpoint
	err := database.QueryRow(
		"INSERT INTO endpoints (url) VALUES (?) RETURNING id, url, created_at",
		url,
	).Scan(&ep.ID, &ep.URL, &ep.CreatedAt)
	if err != nil {
		return Endpoint{}, err
	}
	return ep, nil
}

func DeleteEndpoint(database *sql.DB, id int64) error {
	result, err := database.Exec("DELETE FROM endpoints WHERE id = ?", id)
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
