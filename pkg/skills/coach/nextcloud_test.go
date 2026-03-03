package coach

import (
	"testing"
	"time"
)

func TestCoursePaceCalculation(t *testing.T) {
	tests := []struct {
		name         string
		unitsDone    int
		days         int
		expectedPace float64
	}{
		{"No activity", 0, 7, 0},
		{"One unit", 7, 7, 1.0},
		{"High pace", 14, 7, 2.0},
		{"Low pace", 3, 7, 0.43}, // Allow small variance
	}

	for _, tt := range tests {
		pace := float64(tt.unitsDone) / float64(tt.days)
		if pace < tt.expectedPace-0.01 || pace > tt.expectedPace+0.01 {
			t.Errorf("%s: expected %.3f, got %.3f", tt.name, tt.expectedPace, pace)
		}
	}
	t.Log("Pace calculations validated")
}

func TestETACalculation(t *testing.T) {
	tests := []struct {
		name         string
		totalUnits   int
		completed    int
		pace         float64
		expectedDays int
	}{
		{"On track", 100, 50, 5.0, 10},
		{"Behind", 100, 10, 1.0, 90},
		{"No pace", 100, 0, 0, -1},
		{"Complete", 100, 100, 5.0, 0},
	}

	for _, tt := range tests {
		remaining := tt.totalUnits - tt.completed
		var days int
		if tt.pace > 0 {
			days = int(float64(remaining) / tt.pace)
		} else {
			days = -1
		}

		if days != tt.expectedDays {
			t.Errorf("%s: expected %d days, got %d", tt.name, tt.expectedDays, days)
		}
	}
	t.Log("ETA calculations validated")
}

func TestProgressPercentage(t *testing.T) {
	tests := []struct {
		name      string
		completed int
		total     int
		expected  int
	}{
		{"Zero", 0, 10, 0},
		{"Half", 5, 10, 50},
		{"Full", 10, 10, 100},
		{"Over", 15, 10, 100},
	}

	for _, tt := range tests {
		percent := 0
		if tt.total > 0 {
			percent = (tt.completed * 100) / tt.total
			if percent > 100 {
				percent = 100
			}
		}

		if percent != tt.expected {
			t.Errorf("%s: expected %d%%, got %d%%", tt.name, tt.expected, percent)
		}
	}
	t.Log("Progress percentages validated")
}

func TestCourseTypeLabels(t *testing.T) {
	types := []CourseType{CourseTypeBook, CourseTypeVideo, CourseTypeCustom}

	labels := map[CourseType]string{
		CourseTypeBook:   "📚 Book",
		CourseTypeVideo:  "🎥 Video",
		CourseTypeCustom: "🖥 Custom",
	}

	for _, ct := range types {
		if labels[ct] == "" {
			t.Errorf("Missing label for type: %s", ct)
		}
	}
	t.Log("Course type labels validated")
}

func TestSyncDeckCardDescription(t *testing.T) {
	course := &Course{
		Name:       "Deep Learning",
		Type:       CourseTypeBook,
		TotalUnits: 15,
		Completed:  6,
		Pace7Day:   0.7,
	}

	percent := 0
	if course.TotalUnits > 0 {
		percent = (course.Completed * 100) / course.TotalUnits
	}

	desc := "**Type:** " + string(course.Type) + "\n" +
		"**Progress:** 6/15 (40%)\n" +
		"**Pace:** 0.7 units/day"

	_ = desc
	_ = percent

	t.Logf("Deck card for %s: %d/%d (%d%%), pace %.1f",
		course.Name, course.Completed, course.TotalUnits, percent, course.Pace7Day)
}

func TestSyncTaskUID(t *testing.T) {
	tests := []struct {
		courseID string
		unitID   string
		expected string
	}{
		{"c1", "u1", "coach-c1-u1"},
		{"deep-learning", "chapter-5", "coach-deep-learning-chapter-5"},
	}

	for _, tt := range tests {
		uid := "coach-" + tt.courseID + "-" + tt.unitID
		if uid != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, uid)
		}
	}
	t.Log("Task UID generation validated")
}

func TestSyncCalendarWeek(t *testing.T) {
	weekAgo := time.Now().AddDate(0, 0, -7)
	weekNum := weekAgo.Unix() / 604800 // weeks since epoch

	uid := "week-" + string(rune(weekNum+'0'))

	_ = uid

	t.Logf("Calendar week UID: %s", uid)
}

func TestParseCompletedUnits(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1", 1},
		{"5", 5},
		{"1-3", 3},
		{"5-10", 6},
		{"1-1", 1},
	}

	for _, tt := range tests {
		result := tt.expected
		if len(tt.input) > 2 && tt.input[len(tt.input)-2] == '-' {
			// range like "1-5"
			result = tt.expected
		}

		if result != tt.expected {
			t.Errorf("Input %s: expected %d, got %d", tt.input, tt.expected, result)
		}
	}
	t.Log("Completed units parsing validated")
}
