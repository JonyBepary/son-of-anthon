package coach

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/sqlite"
)

func TestSQLiteConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping SQLite database: %v", err)
	}

	t.Log("SQLite connection successful")
}

func TestStreaksTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "momentum.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	createTableSQL := `CREATE TABLE IF NOT EXISTS streaks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		category TEXT UNIQUE NOT NULL,
		current_streak INTEGER DEFAULT 0,
		last_completed_date TEXT
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		t.Fatalf("Failed to create streaks table: %v", err)
	}

	insertSQL := `INSERT INTO streaks (category, current_streak, last_completed_date) VALUES (?, ?, ?)`
	_, err = db.Exec(insertSQL, "IELTS", 5, "2026-02-26")
	if err != nil {
		t.Fatalf("Failed to insert streak: %v", err)
	}

	var category string
	var streak int
	var lastDate string
	err = db.QueryRow("SELECT category, current_streak, last_completed_date FROM streaks WHERE category = ?", "IELTS").Scan(&category, &streak, &lastDate)
	if err != nil {
		t.Fatalf("Failed to query streak: %v", err)
	}

	if category != "IELTS" {
		t.Errorf("Expected category 'IELTS', got '%s'", category)
	}
	if streak != 5 {
		t.Errorf("Expected streak 5, got %d", streak)
	}
	if lastDate != "2026-02-26" {
		t.Errorf("Expected lastDate '2026-02-26', got '%s'", lastDate)
	}

	t.Log("Streaks table operations successful")
}

func TestGlebarezSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "glebarez_test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open with glebarez/sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = db.Exec("INSERT INTO test (name) VALUES (?)", "hello")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	var name string
	err = db.QueryRow("SELECT name FROM test WHERE id = 1").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	if name != "hello" {
		t.Errorf("Expected 'hello', got '%s'", name)
	}

	t.Log("glebarez/sqlite driver works correctly")
}

func TestMultipleInserts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "multi.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	db.Exec("CREATE TABLE habits (id INTEGER PRIMARY KEY, name TEXT)")

	habits := []string{"Exercise", "Reading", "Meditation", "Coding"}
	for _, habit := range habits {
		_, err := db.Exec("INSERT INTO habits (name) VALUES (?)", habit)
		if err != nil {
			t.Fatalf("Failed to insert %s: %v", habit, err)
		}
	}

	rows, err := db.Query("SELECT name FROM habits")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name string
		rows.Scan(&name)
		count++
	}

	if count != 4 {
		t.Errorf("Expected 4 habits, got %d", count)
	}

	t.Log("Multiple inserts work correctly")
}

func TestConcurrently(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	defer db.Close()

	db.Exec("CREATE TABLE counters (id INTEGER PRIMARY KEY, value INTEGER)")

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			db.Exec("INSERT INTO counters (value) VALUES (?)", n)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM counters").Scan(&count)

	t.Logf("Concurrent insert test: inserted %d rows", count)
}
