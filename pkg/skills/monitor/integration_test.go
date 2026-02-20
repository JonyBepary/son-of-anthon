//go:build integration
// +build integration

package monitor

import (
	"testing"
	"time"
)

func TestLiveFeedFetch(t *testing.T) {
	skill := newTestSkill(t)

	feeds := []struct {
		name, url, category string
	}{
		{"arXiv AI", "https://rss.arxiv.org/rss/cs.AI", "research"},
		{"HuggingFace", "https://huggingface.co/blog/rss", "research"},
	}

	for _, f := range feeds {
		t.Run(f.name, func(t *testing.T) {
			feed := Feed{
				Name:     f.name,
				URL:      f.url,
				Category: f.category,
				Tier:     1,
			}
			items, err := skill.fetchFeed(feed)
			if err != nil {
				t.Skipf("Skipping %s: feed unavailable: %v", f.name, err)
			}
			if len(items) == 0 {
				t.Errorf("%s returned 0 items — feed may be down", f.name)
			}
			t.Logf("%s: fetched %d items", f.name, len(items))
		})
	}
}

func TestDoubleFetchDeduplication(t *testing.T) {
	skill := newTestSkill(t)

	// Use HuggingFace blog - posts ~2x/week, stable, low volume
	// This feed will genuinely return 0 new items on second fetch
	url := "https://huggingface.co/blog/feed.xml"

	feed := Feed{
		Name:     "HuggingFace Blog",
		URL:      url,
		Category: "research",
		Tier:     1,
	}

	items1, err := skill.fetchFeed(feed)
	if err != nil {
		t.Fatalf("First fetch failed: %v", err)
	}
	t.Logf("First fetch: %d new items", len(items1))

	for _, item := range items1 {
		skill.markSeen(&item)
	}

	time.Sleep(2 * time.Second)

	items2, err := skill.fetchFeed(feed)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}
	t.Logf("Second fetch: %d new items (should be 0 or close to 0)", len(items2))

	newItems := 0
	for _, item := range items2 {
		if !skill.isDuplicateURL(&item) {
			newItems++
		}
	}

	if newItems > 2 {
		t.Errorf("Second fetch produced %d items — dedup is not working. Expected 0-2 max", newItems)
	}
}
