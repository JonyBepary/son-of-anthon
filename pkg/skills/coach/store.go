package coach

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/sqlite"
)

type CourseType string

const (
	CourseTypeBook   CourseType = "book"
	CourseTypeVideo  CourseType = "video"
	CourseTypeCustom CourseType = "custom"
)

type Course struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       CourseType `json:"type"`
	Source     string     `json:"source"`
	TotalUnits int        `json:"total_units"`
	Completed  int        `json:"completed"`
	Pace7Day   float64    `json:"pace_7day"`
	Status     string     `json:"status"` // active, paused, archived
	TargetDate *time.Time `json:"target_date,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Unit struct {
	ID          string     `json:"id"`
	CourseID    string     `json:"course_id"`
	Index       int        `json:"index"`
	Title       string     `json:"title"`
	Duration    int        `json:"duration"` // minutes
	Status      string     `json:"status"`   // pending, completed, skipped
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Session struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	UnitsDone int       `json:"units_done"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

type CoachStore struct {
	db *sql.DB
}

func NewCoachStore(workspace string) (*CoachStore, error) {
	dbPath := filepath.Join(workspace, "coach.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS courses (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			source TEXT,
			total_units INTEGER NOT NULL,
			completed INTEGER DEFAULT 0,
			pace_7day REAL DEFAULT 0,
			status TEXT DEFAULT 'active',
			target_date INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS units (
			id TEXT PRIMARY KEY,
			course_id TEXT NOT NULL,
			index_num INTEGER NOT NULL,
			title TEXT NOT NULL,
			duration INTEGER DEFAULT 0,
			status TEXT DEFAULT 'pending',
			completed_at INTEGER,
			FOREIGN KEY (course_id) REFERENCES courses(id)
		);

		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			course_id TEXT NOT NULL,
			units_done INTEGER NOT NULL,
			started_at INTEGER NOT NULL,
			ended_at INTEGER NOT NULL,
			FOREIGN KEY (course_id) REFERENCES courses(id)
		);

		CREATE INDEX IF NOT EXISTS idx_units_course ON units(course_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_course ON sessions(course_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
	`)
	if err != nil {
		return nil, err
	}

	return &CoachStore{db: db}, nil
}

func (s *CoachStore) CreateCourse(c *Course) error {
	_, err := s.db.Exec(`
		INSERT INTO courses (id, name, type, source, total_units, completed, pace_7day, status, target_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Name, c.Type, c.Source, c.TotalUnits, c.Completed, c.Pace7Day, c.Status, unixTime(c.TargetDate), c.CreatedAt.Unix(), c.UpdatedAt.Unix())
	return err
}

func (s *CoachStore) AddUnit(u *Unit) error {
	_, err := s.db.Exec(`
		INSERT INTO units (id, course_id, index_num, title, duration, status, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.CourseID, u.Index, u.Title, u.Duration, u.Status, unixTime(u.CompletedAt))
	return err
}

func (s *CoachStore) CompleteUnit(unitID string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		UPDATE units SET status = 'completed', completed_at = ? WHERE id = ?
	`, now, unitID)
	if err != nil {
		return err
	}
	// Update course completed count
	_, err = s.db.Exec(`
		UPDATE courses SET 
			completed = (SELECT COUNT(*) FROM units WHERE course_id = (SELECT course_id FROM units WHERE id = ?) AND status = 'completed'),
			updated_at = ?
		WHERE id = (SELECT course_id FROM units WHERE id = ?)
	`, unitID, now, unitID)
	return err
}

func (s *CoachStore) AddSession(session *Session) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (id, course_id, units_done, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?)
	`, session.ID, session.CourseID, session.UnitsDone, session.StartedAt.Unix(), session.EndedAt.Unix())
	if err != nil {
		return err
	}
	// Recalculate 7-day pace
	return s.updatePace(session.CourseID)
}

func (s *CoachStore) updatePace(courseID string) error {
	weekAgo := time.Now().AddDate(0, 0, -7).Unix()
	var total int
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(units_done), 0) FROM sessions 
		WHERE course_id = ? AND started_at >= ?
	`, courseID, weekAgo).Scan(&total)
	if err != nil {
		return err
	}
	pace := float64(total) / 7.0
	_, err = s.db.Exec(`UPDATE courses SET pace_7day = ? WHERE id = ?`, pace, courseID)
	return err
}

func (s *CoachStore) GetCourse(id string) (*Course, error) {
	c := &Course{}
	var targetDate *int64
	var createdAt, updatedAt int64
	err := s.db.QueryRow(`
		SELECT id, name, type, source, total_units, completed, pace_7day, status, target_date, created_at, updated_at
		FROM courses WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Type, &c.Source, &c.TotalUnits, &c.Completed, &c.Pace7Day, &c.Status, &targetDate, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	if targetDate != nil {
		t := time.Unix(*targetDate, 0)
		c.TargetDate = &t
	}
	return c, nil
}

func (s *CoachStore) GetActiveCourses() ([]*Course, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, source, total_units, completed, pace_7day, status, target_date, created_at, updated_at
		FROM courses WHERE status = 'active' ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []*Course
	for rows.Next() {
		c := &Course{}
		var targetDate *int64
		var createdAt, updatedAt int64
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Source, &c.TotalUnits, &c.Completed, &c.Pace7Day, &c.Status, &targetDate, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		if targetDate != nil {
			t := time.Unix(*targetDate, 0)
			c.TargetDate = &t
		}
		courses = append(courses, c)
	}
	return courses, nil
}

func (s *CoachStore) GetCourseUnits(courseID string) ([]*Unit, error) {
	rows, err := s.db.Query(`
		SELECT id, course_id, index_num, title, duration, status, completed_at
		FROM units WHERE course_id = ? ORDER BY index_num
	`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []*Unit
	for rows.Next() {
		u := &Unit{}
		var completedAt *int64
		if err := rows.Scan(&u.ID, &u.CourseID, &u.Index, &u.Title, &u.Duration, &u.Status, &completedAt); err != nil {
			return nil, err
		}
		if completedAt != nil {
			t := time.Unix(*completedAt, 0)
			u.CompletedAt = &t
		}
		units = append(units, u)
	}
	return units, nil
}

func (s *CoachStore) GetWeeklyStats() (map[string]interface{}, error) {
	weekAgo := time.Now().AddDate(0, 0, -7).Unix()
	var totalUnits int
	var totalSessions int
	var totalTime int

	s.db.QueryRow(`SELECT COALESCE(SUM(units_done), 0) FROM sessions WHERE started_at >= ?`, weekAgo).Scan(&totalUnits)
	s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE started_at >= ?`, weekAgo).Scan(&totalSessions)
	s.db.QueryRow(`
		SELECT COALESCE(SUM(ended_at - started_at), 0) / 60 FROM sessions WHERE started_at >= ?
	`, weekAgo).Scan(&totalTime)

	return map[string]interface{}{
		"units_completed": totalUnits,
		"sessions":        totalSessions,
		"minutes":         totalTime,
		"days":            7,
	}, nil
}

func (s *CoachStore) GetCourseProgress(courseID string) (map[string]interface{}, error) {
	c, err := s.GetCourse(courseID)
	if err != nil {
		return nil, err
	}

	units, err := s.GetCourseUnits(courseID)
	if err != nil {
		return nil, err
	}

	completed := 0
	for _, u := range units {
		if u.Status == "completed" {
			completed++
		}
	}

	percent := 0
	if c.TotalUnits > 0 {
		percent = (completed * 100) / c.TotalUnits
	}

	// Calculate ETA
	daysLeft := 0
	if c.Pace7Day > 0 {
		remaining := c.TotalUnits - completed
		daysLeft = int(float64(remaining) / c.Pace7Day)
	}

	return map[string]interface{}{
		"course":    c.Name,
		"completed": completed,
		"total":     c.TotalUnits,
		"percent":   percent,
		"pace_7day": c.Pace7Day,
		"days_left": daysLeft,
		"units":     units,
	}, nil
}

func unixTime(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}

func (s *CoachStore) Close() error {
	return s.db.Close()
}

// LLM-friendly JSON formats (token efficient)

func (s *CoachStore) ActiveCoursesJSON() (string, error) {
	courses, err := s.GetActiveCourses()
	if err != nil {
		return "", err
	}
	if len(courses) == 0 {
		return "No active courses", nil
	}

	var result string
	for _, c := range courses {
		percent := 0
		if c.TotalUnits > 0 {
			percent = (c.Completed * 100) / c.TotalUnits
		}
		result += fmt.Sprintf("%s | %d/%d (%d%%) | pace: %.1f/day\n", c.Name, c.Completed, c.TotalUnits, percent, c.Pace7Day)
	}
	return result, nil
}

func (s *CoachStore) CourseProgressJSON(courseName string) (string, error) {
	// Find course by name prefix match
	courses, err := s.GetActiveCourses()
	if err != nil {
		return "", err
	}

	var target *Course
	for _, c := range courses {
		if len(courseName) >= 3 && len(c.Name) >= 3 &&
			(c.Name[:min(3, len(c.Name))] == courseName[:min(3, len(courseName))] ||
				containsIgnoreCase(c.Name, courseName)) {
			target = c
			break
		}
	}
	if target == nil {
		return fmt.Sprintf("Course not found: %s", courseName), nil
	}

	progress, err := s.GetCourseProgress(target.ID)
	if err != nil {
		return "", err
	}

	units := progress["units"].([]*Unit)
	var unitList string
	for _, u := range units {
		status := "○"
		if u.Status == "completed" {
			status = "✓"
		} else if u.Status == "skipped" {
			status = "✗"
		}
		unitList += fmt.Sprintf("  %s %s\n", status, u.Title)
	}

	return fmt.Sprintf("%s: %d/%d (%d%%)\nPace: %.1f units/day\nUnits:\n%s",
		progress["course"], progress["completed"], progress["total"], progress["percent"],
		progress["pace_7day"], unitList), nil
}

func (s *CoachStore) WeeklyStatsJSON() (string, error) {
	stats, err := s.GetWeeklyStats()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("This week: %d units, %d sessions, %d minutes",
		stats["units_completed"], stats["sessions"], stats["minutes"]), nil
}

func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
