package monitor

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hbollon/go-edlib"
)

func newTestSkill(t *testing.T) *MonitorSkill {
	t.Helper()
	skill := NewSkill()
	return skill
}

func makeItem(url, title string) *NewsItem {
	return &NewsItem{
		URL:          url,
		CanonicalURL: canonicalizeURL(url),
		TitleRaw:     title,
		TitleNormal:  normalizeTitle(title),
		BodyHash:     hashText(title),
		PublishedAt:  time.Now(),
		IngestedAt:   time.Now(),
		SourceTier:   1,
		SourceLang:   "en",
		Category:     "general",
	}
}

func makeItemWithBody(url, title, body string) *NewsItem {
	item := makeItem(url, title)
	item.Summary = body
	item.BodyHash = hashText(body)
	return item
}

func makeItemWithCategory(url, title, category string) *NewsItem {
	item := makeItem(url, title)
	item.Category = category
	return item
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

func (s *MonitorSkill) isDuplicateURL(item *NewsItem) bool {
	canon := item.CanonicalURL
	if canon == "" {
		canon = canonicalizeURL(item.URL)
	}
	if _, ok := s.seenURLs[canon]; ok {
		return true
	}
	if s.db != nil {
		entries := s.db.GetDedupCache("url")
		for _, e := range entries {
			if e.Hash == canon {
				return true
			}
		}
	}
	return false
}

func (s *MonitorSkill) isDuplicateBody(item *NewsItem) bool {
	if _, ok := s.seenBodies[item.BodyHash]; ok {
		return true
	}
	if s.db != nil {
		entries := s.db.GetDedupCache("body")
		for _, e := range entries {
			if e.Hash == item.BodyHash {
				return true
			}
		}
	}
	return false
}

func (s *MonitorSkill) isDuplicateTitle(item *NewsItem) bool {
	window := s.timeWindows[item.Category]
	if window == 0 {
		window = s.timeWindows["default"]
	}
	itemTime := item.PublishedAt
	if itemTime.IsZero() {
		itemTime = time.Now()
	}

	for normalizedTitle, seenTime := range s.seenTitles {
		timeDiff := itemTime.Sub(seenTime)
		if timeDiff < 0 {
			timeDiff = -timeDiff
		}
		if timeDiff > window {
			continue
		}
		score := computeSimilarity(item.TitleNormal, normalizedTitle)
		if score >= float32(FuzzyThreshold) {
			return true
		}
	}
	if s.db != nil {
		entries := s.db.GetDedupCache("title")
		for _, e := range entries {
			timeDiff := itemTime.Sub(e.SeenAt)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			if timeDiff > window {
				continue
			}
			score := computeSimilarity(item.TitleNormal, e.Hash)
			if score >= float32(FuzzyThreshold) {
				return true
			}
		}
	}
	return false
}

func computeSimilarity(s1, s2 string) float32 {
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if hasDifferentNumbers(words1, words2) {
		return 0
	}

	sorted1 := sortWords(words1)
	sorted2 := sortWords(words2)
	joined1 := strings.Join(sorted1, " ")
	joined2 := strings.Join(sorted2, " ")

	if joined1 == joined2 {
		return 100
	}

	jaroWinkler, _ := edlib.StringsSimilarity(joined1, joined2, edlib.JaroWinkler)
	if jaroWinkler >= 0.80 {
		return jaroWinkler * 100
	}

	jaroWinklerOrig, _ := edlib.StringsSimilarity(s1, s2, edlib.JaroWinkler)
	levenshteinNorm := 1.0 - float32(edlib.LevenshteinDistance(s1, s2))/float32(maxInt(len(s1), len(s2)))
	jaccard := edlib.JaccardSimilarity(s1, s2, 2)

	maxScore := jaroWinklerOrig
	if levenshteinNorm > maxScore {
		maxScore = levenshteinNorm
	}
	if jaccard > maxScore {
		maxScore = jaccard
	}

	return maxScore * 100
}

func sortWords(words []string) []string {
	sorted := make([]string, len(words))
	copy(sorted, words)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

func computeTokenOverlap(words1, words2 []string) float32 {
	if len(words1) == 0 || len(words2) == 0 {
		return 0
	}

	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}
	for _, w := range words2 {
		set2[w] = true
	}

	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	minLen := len(words1)
	if len(words2) < minLen {
		minLen = len(words2)
	}
	if minLen == 0 {
		return 0
	}

	return float32(intersection) / float32(minLen)
}

func hasDifferentNumbers(words1, words2 []string) bool {
	numPattern := regexp.MustCompile(`\d+`)
	nums1 := numPattern.FindAllString(strings.Join(words1, " "), -1)
	nums2 := numPattern.FindAllString(strings.Join(words2, " "), -1)

	if len(nums1) == 0 && len(nums2) == 0 {
		return false
	}

	if len(nums1) != len(nums2) {
		return true
	}

	for i := range nums1 {
		if nums1[i] != nums2[i] {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *MonitorSkill) processItem(item *NewsItem) bool {
	if s.isDuplicateURL(item) {
		return true
	}
	if s.isDuplicateBody(item) {
		return true
	}
	if s.isDuplicateTitle(item) {
		return true
	}
	s.markSeen(item)
	return false
}

func (s *MonitorSkill) insertExpiredCacheEntry(url, hashType string) {
	if s.db != nil {
		expired := time.Now().Add(-8 * 24 * time.Hour)
		s.db.InsertDedupCache(hashType, url, expired, expired)
	}
}

func (s *MonitorSkill) close() {
	if s.db != nil {
		s.db.Close()
	}
}
