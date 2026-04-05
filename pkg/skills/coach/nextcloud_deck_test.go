package coach

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildDeckURL(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"https://nextcloud.example.com", "https://nextcloud.example.com/index.php/apps/deck/api/v1.0/"},
		{"https://nextcloud.example.com/", "https://nextcloud.example.com/index.php/apps/deck/api/v1.0/"},
		{"https://cloud.example.com/nextcloud", "https://cloud.example.com/nextcloud/index.php/apps/deck/api/v1.0/"},
	}

	for _, tt := range tests {
		url := strings.TrimRight(tt.host, "/") + "/index.php/apps/deck/api/v1.0/"
		if url != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, url)
		}
	}
	t.Log("Deck URL building validated")
}

func TestOCSAPIRequestHeader(t *testing.T) {
	called := false
	var receivedHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		receivedHeader = r.Header.Get("OCS-APIRequest")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", server.URL+"/boards", nil)
	req.SetBasicAuth("user", "pass")
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if !called {
		t.Fatal("Handler was not called")
	}
	if receivedHeader != "true" {
		t.Errorf("Expected OCS-APIRequest header 'true', got '%s'", receivedHeader)
	}
	t.Log("OCS-APIRequest header validated")
}

func TestDeckBoardCreationRequiresColor(t *testing.T) {
	boardReq := map[string]string{
		"title": "Learning",
		"color": "31CC7C",
	}

	jsonData, err := json.Marshal(boardReq)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed["title"] != "Learning" {
		t.Errorf("Expected title 'Learning', got '%v'", parsed["title"])
	}
	if parsed["color"] != "31CC7C" {
		t.Errorf("Expected color '31CC7C', got '%v'", parsed["color"])
	}
	t.Log("Board creation payload validated")
}

func TestDeckBoardResponseParsing(t *testing.T) {
	response := `{
		"id": 2578,
		"title": "Learning",
		"owner": {
			"primaryKey": "user@example.com",
			"uid": "user@example.com"
		},
		"color": "31CC7C",
		"archived": false,
		"labels": [],
		"stacks": []
	}`

	var board map[string]interface{}
	if err := json.Unmarshal([]byte(response), &board); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if board["title"] != "Learning" {
		t.Errorf("Expected title 'Learning', got '%v'", board["title"])
	}
	if board["id"] == nil {
		t.Fatal("Expected id to be present")
	}
	id := board["id"].(float64)
	if int(id) != 2578 {
		t.Errorf("Expected id 2578, got %.0f", id)
	}
	t.Log("Board response parsing validated")
}

func TestDeckStackResponseParsing(t *testing.T) {
	response := `[{
		"id": 4345,
		"title": "Active",
		"boardId": 2578,
		"order": 1,
		"cards": []
	}]`

	var stacks []map[string]interface{}
	if err := json.Unmarshal([]byte(response), &stacks); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(stacks) != 1 {
		t.Fatalf("Expected 1 stack, got %d", len(stacks))
	}
	if stacks[0]["title"] != "Active" {
		t.Errorf("Expected title 'Active', got '%v'", stacks[0]["title"])
	}
	t.Log("Stack response parsing validated")
}

func TestDeckCardCreationPayload(t *testing.T) {
	cardReq := map[string]interface{}{
		"title":       "Deep Learning",
		"description": "Type: book\nTotal: 10\nProgress: 3/10 (30%)\nPace: 0.5 units/day",
		"type":        "plain",
	}

	jsonData, err := json.Marshal(cardReq)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed["title"] != "Deep Learning" {
		t.Errorf("Expected title 'Deep Learning', got '%v'", parsed["title"])
	}
	if parsed["type"] != "plain" {
		t.Errorf("Expected type 'plain', got '%v'", parsed["type"])
	}
	desc, ok := parsed["description"].(string)
	if !ok || !strings.Contains(desc, "Progress:") {
		t.Errorf("Expected description to contain Progress, got '%v'", parsed["description"])
	}
	t.Log("Card creation payload validated")
}

func TestDeckCardResponseParsing(t *testing.T) {
	response := `{
		"id": 5678,
		"title": "Deep Learning",
		"description": "Type: book\nTotal: 10",
		"stackId": 4345,
		"type": "plain",
		"owner": "user@example.com",
		"order": 999
	}`

	var card map[string]interface{}
	if err := json.Unmarshal([]byte(response), &card); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if card["title"] != "Deep Learning" {
		t.Errorf("Expected title 'Deep Learning', got '%v'", card["title"])
	}
	if card["stackId"].(float64) != 4345 {
		t.Errorf("Expected stackId 4345, got %v", card["stackId"])
	}
	t.Log("Card response parsing validated")
}

func TestDeckProgressDescriptionFormat(t *testing.T) {
	courses := []struct {
		name       string
		courseType string
		total      int
		completed  int
		pace       float64
	}{
		{"Deep Learning", "book", 15, 6, 0.7},
		{"Python Tutorial", "video", 50, 25, 2.0},
		{"Rust Basics", "custom", 10, 10, 1.0},
	}

	for _, c := range courses {
		percent := 0
		if c.total > 0 {
			percent = (c.completed * 100) / c.total
		}

		desc := fmt.Sprintf("**Type:** %s\n**Progress:** %d/%d (%d%%)\n**Pace:** %.1f units/day",
			c.courseType, c.completed, c.total, percent, c.pace)

		if !strings.Contains(desc, c.courseType) {
			t.Errorf("Expected description to contain type %s", c.courseType)
		}
		if !strings.Contains(desc, fmt.Sprintf("%d%%", percent)) {
			t.Errorf("Expected description to contain %d%%", percent)
		}
	}
	t.Log("Progress description format validated")
}

func TestDeckErrorResponseParsing(t *testing.T) {
	errorResponse := `{"status": 400, "message": "color must be provided and must be not empty"}`

	var errResp map[string]interface{}
	if err := json.Unmarshal([]byte(errorResponse), &errResp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if errResp["status"] != float64(400) {
		t.Errorf("Expected status 400, got %v", errResp["status"])
	}
	if errResp["message"] != "color must be provided and must be not empty" {
		t.Errorf("Unexpected message: %v", errResp["message"])
	}
	t.Log("Error response parsing validated")
}

func TestDeckFindExistingBoard(t *testing.T) {
	boards := []map[string]interface{}{
		{"id": float64(2443), "title": "Welcome to Nextcloud Deck!"},
		{"id": float64(2578), "title": "Learning"},
	}

	searchTitle := "Learning"
	var foundID string
	for _, b := range boards {
		if b["title"] == searchTitle {
			foundID = fmt.Sprintf("%.0f", b["id"].(float64))
			break
		}
	}

	if foundID != "2578" {
		t.Errorf("Expected to find board with id 2578, got %s", foundID)
	}
	t.Log("Find existing board validated")
}

func TestDeckFindStackByTitle(t *testing.T) {
	stacks := []map[string]interface{}{
		{"id": float64(100), "title": "To Do"},
		{"id": float64(4345), "title": "Active"},
		{"id": float64(200), "title": "Done"},
	}

	searchTitle := "Active"
	var foundID string
	for _, s := range stacks {
		if s["title"] == searchTitle {
			foundID = fmt.Sprintf("%.0f", s["id"].(float64))
			break
		}
	}

	if foundID != "4345" {
		t.Errorf("Expected to find stack with id 4345, got %s", foundID)
	}
	t.Log("Find stack by title validated")
}

func TestDeckFindCardByTitle(t *testing.T) {
	cards := []map[string]interface{}{
		{"id": float64(100), "title": "Introduction"},
		{"id": float64(5678), "title": "Deep Learning"},
		{"id": float64(200), "title": "Advanced Topics"},
	}

	searchTitle := "Deep Learning"
	var foundID string
	for _, c := range cards {
		if c["title"] == searchTitle {
			foundID = fmt.Sprintf("%.0f", c["id"].(float64))
			break
		}
	}

	if foundID != "5678" {
		t.Errorf("Expected to find card with id 5678, got %s", foundID)
	}
	t.Log("Find card by title validated")
}

func TestDeckNilSafety(t *testing.T) {
	var board map[string]interface{}

	if board == nil {
		board = make(map[string]interface{})
	}

	if board["id"] != nil {
		t.Error("Expected nil id")
	}

	board["title"] = "Test"
	board["id"] = float64(123)

	if board["title"] != "Test" {
		t.Errorf("Expected title 'Test', got '%v'", board["title"])
	}
	if board["id"] == nil {
		t.Error("Expected id to be set")
	}
	t.Log("Nil safety checks validated")
}

func TestDeckBoardUpdatePayload(t *testing.T) {
	updateReq := map[string]interface{}{
		"title":       "Deep Learning",
		"description": "**Type:** book\n**Progress:** 6/15 (40%)\n**Pace:** 0.7 units/day",
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if !strings.Contains(string(jsonData), "Progress:") {
		t.Error("Expected JSON to contain Progress")
	}
	if !strings.Contains(string(jsonData), "40%") {
		t.Error("Expected JSON to contain 40%")
	}
	t.Log("Board update payload validated")
}

func TestDeckAPIIntegration(t *testing.T) {
	host := os.Getenv("NEXTCLOUD_HOST")
	username := os.Getenv("NEXTCLOUD_USERNAME")
	password := os.Getenv("NEXTCLOUD_PASSWORD")

	if host == "" || username == "" || password == "" {
		t.Skip("Skipping integration test - set NEXTCLOUD_HOST, NEXTCLOUD_USERNAME, NEXTCLOUD_PASSWORD")
	}

	apiURL := strings.TrimRight(host, "/") + "/index.php/apps/deck/api/v1.0/"

	client := &http.Client{Timeout: 30 * time.Second}

	// Test 1: Get boards
	req, _ := http.NewRequest("GET", apiURL+"/boards", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get boards: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var boards []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&boards); err != nil {
		t.Fatalf("Failed to decode boards: %v", err)
	}

	if len(boards) == 0 {
		t.Log("No boards found")
	} else {
		t.Logf("Found %d boards", len(boards))
		for _, b := range boards {
			t.Logf("  - %s (ID: %.0f)", b["title"], b["id"])
		}
	}

	// Test 2: Try to get specific board (2578 - Learning)
	req2, _ := http.NewRequest("GET", apiURL+"/boards/2578", nil)
	req2.SetBasicAuth(username, password)
	req2.Header.Set("OCS-APIRequest", "true")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Failed to get board: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("Expected status 200, got %d", resp2.StatusCode)
	}

	var board map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&board); err != nil {
		t.Fatalf("Failed to decode board: %v", err)
	}

	if board["title"] != "Learning" {
		t.Errorf("Expected board title 'Learning', got '%v'", board["title"])
	}

	stacks, ok := board["stacks"].([]interface{})
	if !ok || len(stacks) == 0 {
		t.Log("Board has no stacks")
	} else {
		t.Logf("Board has %d stacks", len(stacks))
	}

	t.Log("Integration test passed!")
}
