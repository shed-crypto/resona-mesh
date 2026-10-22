-- 002_fts5.sql: Full-Text Search index and synchronization triggers for track discovery

CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
    content_id UNINDEXED,
    title,
    artist,
    description,
    tokenize = 'unicode61'
);

-- Trigger: keep tracks_fts synchronized on INSERT
CREATE TRIGGER IF NOT EXISTS trg_tracks_insert AFTER INSERT ON tracks BEGIN
    INSERT INTO tracks_fts(content_id, title, artist, description)
    VALUES (new.content_id, new.title, new.artist, new.description);
END;

-- Trigger: keep tracks_fts synchronized on UPDATE
CREATE TRIGGER IF NOT EXISTS trg_tracks_update AFTER UPDATE ON tracks BEGIN
    DELETE FROM tracks_fts WHERE content_id = old.content_id;
    INSERT INTO tracks_fts(content_id, title, artist, description)
    VALUES (new.content_id, new.title, new.artist, new.description);
END;

-- Trigger: keep tracks_fts synchronized on DELETE
CREATE TRIGGER IF NOT EXISTS trg_tracks_delete AFTER DELETE ON tracks BEGIN
    DELETE FROM tracks_fts WHERE content_id = old.content_id;
END;
