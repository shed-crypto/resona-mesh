package storage

// MigrationInitSQL contains the relational schema for tracks, blocks, and indexes.
const MigrationInitSQL = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tracks (
    content_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    artist TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    lyrics TEXT NOT NULL DEFAULT '',
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
`

// MigrationFTS5SQL contains the FTS5 virtual table and synchronization triggers.
const MigrationFTS5SQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
    content_id UNINDEXED,
    title,
    artist,
    description,
    tokenize = 'unicode61'
);

CREATE TRIGGER IF NOT EXISTS trg_tracks_insert AFTER INSERT ON tracks BEGIN
    INSERT INTO tracks_fts(content_id, title, artist, description)
    VALUES (new.content_id, new.title, new.artist, new.description);
END;

CREATE TRIGGER IF NOT EXISTS trg_tracks_update AFTER UPDATE ON tracks BEGIN
    DELETE FROM tracks_fts WHERE content_id = old.content_id;
    INSERT INTO tracks_fts(content_id, title, artist, description)
    VALUES (new.content_id, new.title, new.artist, new.description);
END;

CREATE TRIGGER IF NOT EXISTS trg_tracks_delete AFTER DELETE ON tracks BEGIN
    DELETE FROM tracks_fts WHERE content_id = old.content_id;
END;
`

// MigrationModerationSQL contains the relational schema for reports, decisions, and blocked content.
const MigrationModerationSQL = `
CREATE TABLE IF NOT EXISTS reports (
    event_id TEXT PRIMARY KEY,
    content_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL DEFAULT '',
    reporter_key TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    description TEXT NOT NULL,
    evidence_ref TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    nonce TEXT NOT NULL,
    signature TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    receipt_signature TEXT NOT NULL DEFAULT '',
    receipt_received_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_reports_content_id ON reports(content_id);
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status);

CREATE TABLE IF NOT EXISTS decisions (
    event_id TEXT PRIMARY KEY,
    schema_version TEXT NOT NULL DEFAULT '1.0.0',
    network_id TEXT NOT NULL DEFAULT 'resonamesh-main',
    policy_id TEXT NOT NULL,
    policy_epoch INTEGER NOT NULL DEFAULT 1,
    content_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL DEFAULT '',
    sequence INTEGER NOT NULL,
    previous_event_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    reason_text TEXT NOT NULL,
    moderator_key TEXT NOT NULL,
    issued_at INTEGER NOT NULL,
    signature TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_decisions_content_id ON decisions(content_id);
CREATE INDEX IF NOT EXISTS idx_decisions_seq ON decisions(policy_id, policy_epoch, sequence);

CREATE TABLE IF NOT EXISTS blocked_content (
    content_id TEXT PRIMARY KEY,
    payload_hash TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT 'BLOCK',
    decision_event_id TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    reason_text TEXT NOT NULL,
    blocked_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_blocked_content_payload ON blocked_content(payload_hash);
`

// MigrationCoverSQL creates the relational table for storing track cover images.
const MigrationCoverSQL = `
CREATE TABLE IF NOT EXISTS track_covers (
    content_id TEXT PRIMARY KEY,
    mime_type TEXT NOT NULL,
    data BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(content_id) REFERENCES tracks(content_id) ON DELETE CASCADE
);
`


// MigrationGovernanceSQL creates tables for delegated moderator certificates and offline key rotations.
const MigrationGovernanceSQL = `
CREATE TABLE IF NOT EXISTS moderator_certificates (
    cert_id TEXT PRIMARY KEY,
    network_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('APPROVE_MODERATOR', 'REVOKE_MODERATOR')),
    moderator_pubkey TEXT NOT NULL,
    alias TEXT NOT NULL DEFAULT '',
    issued_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    root_admin_pubkey TEXT NOT NULL,
    signature TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mod_certs_pubkey ON moderator_certificates(moderator_pubkey, issued_at DESC);

CREATE TABLE IF NOT EXISTS key_rotations (
    event_id TEXT PRIMARY KEY,
    network_id TEXT NOT NULL,
    key_role TEXT NOT NULL CHECK(key_role IN ('ROOT_ADMIN', 'RELEASE')),
    new_policy_epoch INTEGER NOT NULL,
    old_pubkey TEXT NOT NULL,
    new_pubkey TEXT NOT NULL,
    recovery_pubkey TEXT NOT NULL,
    reason TEXT NOT NULL,
    issued_at INTEGER NOT NULL,
    signature TEXT NOT NULL,
    UNIQUE(key_role, new_policy_epoch)
);
`