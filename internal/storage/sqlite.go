package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB manages the SQLite database connection pool and schema migrations.
type DB struct {
	db *sql.DB
}

// Open opens or creates an embedded SQLite database, applies pragmatic settings
// (WAL mode, foreign keys, synchronous normal) and runs pending migrations.
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("storage: failed to create database directory: %w", err)
		}
	}

	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_txlock=immediate", path)
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to open sqlite connection: %w", err)
	}

	// Pool settings optimized for embedded single-process SQLite
	if path == ":memory:" {
		sqlDB.SetMaxOpenConns(1) // in-memory db requires single connection to persist state
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("storage: database ping failed: %w", err)
	}

	instance := &DB{db: sqlDB}
	if err := instance.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("storage: migration failed: %w", err)
	}

	return instance, nil
}

// migrate executes embedded DDL migrations in strict order.
func (d *DB) migrate() error {
	migrations := []string{
		MigrationInitSQL,
		MigrationFTS5SQL,
		MigrationModerationSQL,
		MigrationCoverSQL,
		MigrationGovernanceSQL,
	}

	for idx, m := range migrations {
		if _, err := d.db.Exec(m); err != nil {
			return fmt.Errorf("migration %d failed: %w", idx+1, err)
		}
	}

	// Idempotent column check for existing databases
	if err := d.ensureMetadataColumns(); err != nil {
		return err
	}

	return nil
}

func (d *DB) ensureMetadataColumns() error {
	rows, err := d.db.Query("PRAGMA table_info(tracks)")
	if err != nil {
		return nil
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			cols[name] = true
		}
	}
	if !cols["lyrics"] {
		_, _ = d.db.Exec("ALTER TABLE tracks ADD COLUMN lyrics TEXT NOT NULL DEFAULT ''")
	}
	if !cols["genre"] {
		_, _ = d.db.Exec("ALTER TABLE tracks ADD COLUMN genre TEXT NOT NULL DEFAULT ''")
	}
	if !cols["tags_json"] {
		_, _ = d.db.Exec("ALTER TABLE tracks ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]'")
	}
	if !cols["bpm"] {
		_, _ = d.db.Exec("ALTER TABLE tracks ADD COLUMN bpm INTEGER NOT NULL DEFAULT 0")
	}
	if !cols["mood"] {
		_, _ = d.db.Exec("ALTER TABLE tracks ADD COLUMN mood TEXT NOT NULL DEFAULT ''")
	}
	return nil
}

// Close closes the underlying database connection pool.
func (d *DB) Close() error {
	return d.db.Close()
}

// Raw returns the underlying *sql.DB connection.
func (d *DB) Raw() *sql.DB {
	return d.db
}
