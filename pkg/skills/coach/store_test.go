package coach

import (
	"testing"
	"time"
)

func TestCoachStore(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewCoachStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test CreateCourse
	now := time.Now()
	course := &Course{
		ID:         "test-course-1",
		Name:       "Deep Learning",
		Type:       CourseTypeBook,
		Source:     "manual",
		TotalUnits: 15,
		Completed:  0,
		Pace7Day:   0,
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := store.CreateCourse(course); err != nil {
		t.Fatalf("Failed to create course: %v", err)
	}

	// Test AddUnit
	for i := 1; i <= 15; i++ {
		unit := &Unit{
			ID:       "unit-1-" + string(rune(i+'0')),
			CourseID: "test-course-1",
			Index:    i,
			Title:    "Chapter " + string(rune(i+'0')),
			Duration: 30,
			Status:   "pending",
		}
		if err := store.AddUnit(unit); err != nil {
			t.Fatalf("Failed to add unit: %v", err)
		}
	}

	// Test GetCourse
	c, err := store.GetCourse("test-course-1")
	if err != nil {
		t.Fatalf("Failed to get course: %v", err)
	}
	if c.Name != "Deep Learning" {
		t.Errorf("Expected Deep Learning, got %s", c.Name)
	}
	if c.TotalUnits != 15 {
		t.Errorf("Expected 15 units, got %d", c.TotalUnits)
	}

	// Test CompleteUnit
	if err := store.CompleteUnit("unit-1-1"); err != nil {
		t.Fatalf("Failed to complete unit: %v", err)
	}

	// Test AddSession
	session := &Session{
		ID:        "session-1",
		CourseID:  "test-course-1",
		UnitsDone: 1,
		StartedAt: now,
		EndedAt:   now,
	}
	if err := store.AddSession(session); err != nil {
		t.Fatalf("Failed to add session: %v", err)
	}

	// Test GetWeeklyStats
	stats, err := store.GetWeeklyStats()
	if err != nil {
		t.Fatalf("Failed to get weekly stats: %v", err)
	}
	if stats["units_completed"].(int) != 1 {
		t.Errorf("Expected 1 unit completed, got %d", stats["units_completed"])
	}

	// Test GetActiveCourses
	courses, err := store.GetActiveCourses()
	if err != nil {
		t.Fatalf("Failed to get active courses: %v", err)
	}
	if len(courses) != 1 {
		t.Errorf("Expected 1 active course, got %d", len(courses))
	}

	// Test GetCourseProgress
	progress, err := store.GetCourseProgress("test-course-1")
	if err != nil {
		t.Fatalf("Failed to get progress: %v", err)
	}
	if progress["completed"].(int) != 1 {
		t.Errorf("Expected 1 completed, got %d", progress["completed"])
	}
	if progress["percent"].(int) != 6 {
		t.Errorf("Expected 6%%, got %d%%", progress["percent"])
	}

	t.Log("All CoachStore tests passed!")
}

func TestActiveCoursesJSON(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewCoachStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()

	// Create test courses
	courses := []*Course{
		{ID: "c1", Name: "Deep Learning", Type: CourseTypeBook, TotalUnits: 10, Completed: 5, Pace7Day: 0.7, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "c2", Name: "Python Basics", Type: CourseTypeVideo, TotalUnits: 20, Completed: 15, Pace7Day: 1.2, Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: "c3", Name: "Old Course", Type: CourseTypeCustom, TotalUnits: 5, Completed: 5, Pace7Day: 0, Status: "archived", CreatedAt: now, UpdatedAt: now},
	}

	for _, c := range courses {
		store.CreateCourse(c)
	}

	// Should only return active courses
	result, err := store.ActiveCoursesJSON()
	if err != nil {
		t.Fatalf("Failed to get JSON: %v", err)
	}

	// Should contain active courses
	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	// Should NOT contain archived
	if contains(result, "Old Course") {
		t.Error("Should not include archived courses")
	}

	t.Logf("ActiveCoursesJSON: %s", result)
}

func TestCourseProgressJSON(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewCoachStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	course := &Course{
		ID: "test-c1", Name: "Machine Learning", Type: CourseTypeBook,
		TotalUnits: 10, Completed: 3, Pace7Day: 0.5, Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}
	store.CreateCourse(course)

	// Add units
	for i := 1; i <= 10; i++ {
		status := "pending"
		if i <= 3 {
			status = "completed"
		}
		store.AddUnit(&Unit{
			ID: "u1", CourseID: "test-c1", Index: i,
			Title: "Chapter " + string(rune(i+'0')), Status: status,
		})
	}

	result, err := store.CourseProgressJSON("Machine")
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Should find by partial match
	if !contains(result, "Machine Learning") {
		t.Error("Should contain course name")
	}

	t.Logf("CourseProgressJSON: %s", result)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
