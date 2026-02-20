package monitor

import (
	"testing"
	"time"
)

func TestExactURLDedup(t *testing.T) {
	skill := newTestSkill(t)

	item1 := makeItem("https://reuters.com/story/123?utm_source=rss", "Reuters kills bill")
	item2 := makeItem("https://reuters.com/story/123?utm_source=twitter", "Reuters kills bill")

	if skill.isDuplicateURL(item1) {
		t.Error("item1 should not be duplicate on first see")
	}
	skill.markSeen(item1)
	if !skill.isDuplicateURL(item2) {
		t.Error("item2 should be duplicate — same canonical URL")
	}
}

func TestExactBodyDedup(t *testing.T) {
	skill := newTestSkill(t)

	item1 := makeItemWithBody("https://apnews.com/story/abc", "AP headline", "Full AP wire body text here verbatim")
	item2 := makeItemWithBody("https://bbc.com/story/xyz", "BBC headline", "Full AP wire body text here verbatim")

	skill.markSeen(item1)
	if !skill.isDuplicateBody(item2) {
		t.Error("Same body from different URL should be caught as duplicate")
	}
}

func TestBodyHashDifferentContent(t *testing.T) {
	skill := newTestSkill(t)

	item1 := makeItemWithBody("https://reuters.com/a", "Floods kill 12", "12 people died in floods")
	item2 := makeItemWithBody("https://reuters.com/b", "Floods kill 20", "20 people died in floods")

	skill.markSeen(item1)
	if skill.isDuplicateBody(item2) {
		t.Error("Different body content should NOT be duplicate — different death toll is new fact")
	}
}

func TestFuzzyTitleDuplicate(t *testing.T) {
	skill := newTestSkill(t)

	cases := []struct {
		title1, title2 string
		isDup          bool
		reason         string
	}{
		{
			"DeepSeek launches R2 model",
			"R2 model launched by DeepSeek",
			true, "token sort ratio should handle word reordering",
		},
		{
			"Bangladesh floods kill 12 people",
			"Bangladesh floods kill 20 people",
			false, "different numbers = different story",
		},
		{
			"OpenAI releases GPT-5",
			"OpenAI releases GPT-5 to the public",
			true, "same words with minor addition",
		},
		{
			"India launches missile",
			"Pakistan launches missile",
			false, "different entity = different story",
		},
		{
			"NVIDIA announces H200 GPU",
			"NVIDIA H200 GPU announced at conference",
			true, "token sort ratio handles passive voice rewrite",
		},
	}

	for _, tc := range cases {
		score := computeSimilarity(normalizeTitle(tc.title1), normalizeTitle(tc.title2))
		t.Logf("'%s' vs '%s': score = %.2f", tc.title1, tc.title2, score)

		item1 := makeItem("https://a.com/1", tc.title1)
		item2 := makeItem("https://b.com/2", tc.title2)
		item2.PublishedAt = item1.PublishedAt.Add(2 * time.Hour)

		skill.markSeen(item1)
		result := skill.isDuplicateTitle(item2)
		if result != tc.isDup {
			t.Errorf("'%s' vs '%s': got isDup=%v, want %v — %s",
				tc.title1, tc.title2, result, tc.isDup, tc.reason)
		}
	}
}

func TestTimeWindowGating(t *testing.T) {
	skill := newTestSkill(t)

	item1 := makeItemWithCategory("https://reuters.com/a", "Bangladesh floods", "bangladesh")
	item1.PublishedAt = time.Now().Add(-30 * time.Hour)

	item2 := makeItemWithCategory("https://prothomalo.com/b", "Bangladesh floods", "bangladesh")
	item2.PublishedAt = time.Now()

	skill.markSeen(item1)
	if skill.isDuplicateTitle(item2) {
		t.Error("Items outside time window should not be deduplicated — could be a new flood event")
	}
}

func TestTimeWindowBreaking(t *testing.T) {
	skill := newTestSkill(t)

	item1 := makeItemWithCategory("https://reuters.com/a", "US strikes Syria", "breaking")
	item1.PublishedAt = time.Now().Add(-7 * time.Hour)

	item2 := makeItemWithCategory("https://ap.com/b", "US strikes Syria", "breaking")
	item2.PublishedAt = time.Now()

	skill.markSeen(item1)
	if skill.isDuplicateTitle(item2) {
		t.Error("Breaking news items > 6h apart should not be deduplicated")
	}
}

func TestCanonicalizeURL(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{
			"https://reuters.com/story?utm_source=rss&utm_medium=feed",
			"https://reuters.com/story",
		},
		{
			"https://thedailystar.net/article/123#comments",
			"https://thedailystar.net/article/123",
		},
		{
			"https://bdnews24.com/story?ref=homepage&source=rss",
			"https://bdnews24.com/story",
		},
		{
			"https://openai.com/blog/gpt5",
			"https://openai.com/blog/gpt5",
		},
	}

	for _, tc := range cases {
		got := canonicalizeURL(tc.input)
		if got != tc.want {
			t.Errorf("canonicalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
