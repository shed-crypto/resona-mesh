-- 001_init.sql: Core relational schema for tracks, audio blocks, and node state

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tracks (
    content_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    artist TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL,
    bitrate_kbps INTEGER NOT NULL DEFAULT 192,
    total_size_bytes INTEGER NOT NULL,
    block_size INTEGER NOT NULL,
    block_count INTEGER NOT NULL,
    publisher_key TEXT NOT NULL,
    published_at INTEGER NOT NULL,
    license_code TEXT NOT NULL,
    signature TEXT NOT NULL,
    pinned INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS blocks (
    content_id TEXT NOT NULL,
    block_index INTEGER NOT NULL,
    block_hash TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    is_stored INTEGER NOT NULL DEFAULT 0,
    last_accessed_at INTEGER NOT NULL,
    PRIMARY KEY(content_id, block_index),
    FOREIGN KEY(content_id) REFERENCES tracks(content_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_blocks_hash ON blocks(block_hash);
CREATE INDEX IF NOT EXISTS idx_tracks_created_at ON tracks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_blocks_stored ON blocks(is_stored);
