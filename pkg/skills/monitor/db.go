package monitor

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	d := &DB{db: db}
	if err := d.init(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func (db *DB) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS items (
		id TEXT PRIMARY KEY,
		source TEXT,
		source_tier INTEGER,
		category TEXT,
		url TEXT,
		title TEXT,
		summary TEXT,
		published_at INTEGER,
		ingested_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS dedup_cache (
		hash TEXT PRIMARY KEY,
		hash_type TEXT,
		category TEXT,
		seen_at INTEGER,
		expires_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_category ON items(category);
	CREATE INDEX IF NOT EXISTS idx_published ON items(published_at);
	`

	_, err := db.db.Exec(schema)
	return err
}

func (db *DB) CountItems() int {
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count)
	return count
}

func (db *DB) InsertItem(item NewsItem) error {
	_, err := db.db.Exec(`
		INSERT OR IGNORE INTO items (id, source, source_tier, category, url, title, summary, published_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.Source, item.SourceTier, item.Category, item.CanonicalURL, item.TitleRaw,
		item.Summary, item.PublishedAt.Unix(), item.IngestedAt.Unix())
	return err
}

type DedupCacheEntry struct {
	Hash      string
	HashType  string
	SeenAt    time.Time
	ExpiresAt time.Time
}

func (db *DB) InsertDedupCache(hashType, hash string, seenAt, expiresAt time.Time) error {
	_, err := db.db.Exec(`
		INSERT OR REPLACE INTO dedup_cache (hash, hash_type, seen_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, hash, hashType, seenAt.Unix(), expiresAt.Unix())
	return err
}

func (db *DB) GetDedupCache(hashType string) []DedupCacheEntry {
	var entries []DedupCacheEntry
	now := time.Now().Unix()

	rows, err := db.db.Query("SELECT hash, hash_type, seen_at, expires_at FROM dedup_cache WHERE hash_type = ? AND expires_at > ?", hashType, now)
	if err != nil {
		return entries
	}
	defer rows.Close()

	for rows.Next() {
		var entry DedupCacheEntry
		var seenAt, expiresAt int64
		if err := rows.Scan(&entry.Hash, &entry.HashType, &seenAt, &expiresAt); err != nil {
			continue
		}
		entry.SeenAt = time.Unix(seenAt, 0)
		entry.ExpiresAt = time.Unix(expiresAt, 0)
		entries = append(entries, entry)
	}

	return entries
}

func (db *DB) CleanupExpired() error {
	now := time.Now().Unix()
	_, err := db.db.Exec("DELETE FROM dedup_cache WHERE expires_at < ?", now)
	return err
}

func (db *DB) Close() error {
	return db.db.Close()
}
