package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	// Create virtual table with unicode61 tokenizer for Cyrillic support
	schema := `
	CREATE VIRTUAL TABLE tracks_fts USING fts5(
		content_id UNINDEXED,
		title,
		artist,
		description,
		tokenize = 'unicode61'
	);
	`
	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("failed to create FTS5 virtual table: %v", err)
	}

	// Insert Ukrainian test records
	insertQuery := `INSERT INTO tracks_fts (content_id, title, artist, description) VALUES (?, ?, ?, ?)`
	testData := [][]string{
		{"cid-001", "Лекція з моделювання складних систем", "Раченко Олексій", "Аудіозапис лекції для студентів ФІТ КНУ"},
		{"cid-002", "Симфонія простору та часу", "Тарас Петренко", "Експериментальний ембієнт трек"},
		{"cid-003", "Цифрова обробка аудіосигналів", "Олексій Мельник", "Практикум з фільтрації та спектрального аналізу"},
	}

	for _, row := range testData {
		if _, err := db.Exec(insertQuery, row[0], row[1], row[2], row[3]); err != nil {
			log.Fatalf("failed to insert test record: %v", err)
		}
	}

	// Test queries
	testQueries := []struct {
		query       string
		expectedCid string
		description string
	}{
		{"моделюван*", "cid-001", "Prefix search: моделюван*"},
		{"лекція", "cid-001", "Exact word search: лекція"},
		{"симфонія", "cid-002", "Exact word search: симфонія"},
		{"спектрального", "cid-003", "Description search: спектрального"},
	}

	searchStmt, err := db.Prepare("SELECT content_id, title, artist FROM tracks_fts WHERE tracks_fts MATCH ?")
	if err != nil {
		log.Fatalf("failed to prepare search statement: %v", err)
	}
	defer searchStmt.Close()

	for _, tq := range testQueries {
		rows, err := searchStmt.Query(tq.query)
		if err != nil {
			log.Fatalf("search query failed [%s]: %v", tq.query, err)
		}

		found := false
		for rows.Next() {
			var cid, title, artist string
			if err := rows.Scan(&cid, &title, &artist); err != nil {
				rows.Close()
				log.Fatalf("failed to scan row: %v", err)
			}
			if cid == tq.expectedCid {
				found = true
				fmt.Printf("[PASS] %s -> matched: %s - \"%s\" (%s)\n", tq.description, cid, title, artist)
			}
		}
		rows.Close()

		if !found {
			log.Fatalf("[FAIL] %s did not match expected %s", tq.description, tq.expectedCid)
		}
	}

	fmt.Println("\nAll SQLite FTS5 Ukrainian Cyrillic tests PASSED successfully!")
}
