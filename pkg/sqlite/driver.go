package sqlite

import (
	"database/sql"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	registered bool
	mu         sync.Mutex
)

func Open(dsn string) (*sql.DB, error) {
	mu.Lock()
	if !registered {
		registered = true
	}
	mu.Unlock()

	return sql.Open("sqlite", dsn)
}
