package monitor

import (
	"testing"
	"time"
)

func TestDedupSurvivesRestart(t *testing.T) {
	dbPath := t.TempDir() + "/test_dedup.db"

	skill1, err := NewMonitorSkill(dbPath)
	if err != nil {
		t.Fatalf("Failed to create skill1: %v", err)
	}
	item := makeItem("https://reuters.com/story/abc", "Big breaking story")

	isDup := skill1.processItem(item)
	if isDup {
		t.Fatal("Item should NOT be duplicate on first ingestion")
	}
	skill1.close()

	skill2, err := NewMonitorSkill(dbPath)
	if err != nil {
		t.Fatalf("Failed to create skill2: %v", err)
	}
	defer skill2.close()

	sameItem := makeItem("https://reuters.com/story/abc", "Big breaking story")
	isDup = skill2.processItem(sameItem)
	if !isDup {
		t.Fatal("CRITICAL: Dedup cache did not survive restart — item re-surfaced after restart")
	}
}

func TestExpiredCacheNotDuplicate(t *testing.T) {
	dbPath := t.TempDir() + "/test_expiry.db"
	skill, err := NewMonitorSkill(dbPath)
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}
	defer skill.close()

	skill.insertExpiredCacheEntry("https://old-story.com/123", "url")

	item := makeItem("https://old-story.com/123", "Story from 8 days ago re-covered")
	isDup := skill.processItem(item)
	if isDup {
		t.Error("Expired cache entry should not block new items — TTL should have cleared it")
	}
}

func TestDifferentCategoryDifferentWindow(t *testing.T) {
	skill, err := NewMonitorSkill("")
	if err != nil {
		t.Fatalf("Failed to create skill: %v", err)
	}

	arxivItem := makeItemWithCategory("https://arxiv.org/abs/2501.001",
		"Attention is all you need — new variant", "research")
	techCrunchItem := makeItemWithCategory("https://techcrunch.com/paper-review",
		"Attention is all you need — new variant", "research")
	techCrunchItem.PublishedAt = arxivItem.PublishedAt.Add(5 * 24 * time.Hour)

	skill.markSeen(arxivItem)
	if !skill.isDuplicateTitle(techCrunchItem) {
		t.Error("Research category has 7d window — same paper after 5 days should still dedup")
	}
}
