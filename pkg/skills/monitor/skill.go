package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hbollon/go-edlib"
	"github.com/mmcdole/gofeed"
)

// Constants
const (
	FuzzyThreshold     = 80
	MaxConcurrentFetch = 5
	TimeWindowBreaking = 6 * time.Hour
	TimeWindowBD       = 24 * time.Hour
	TimeWindowAI       = 48 * time.Hour
	TimeWindowResearch = 7 * 24 * time.Hour
)

// NewsItem - normalized news article
type NewsItem struct {
	ID           string
	Source       string
	SourceTier   int
	SourceLang   string
	Category     string
	URL          string
	CanonicalURL string
	TitleRaw     string
	TitleNormal  string
	Summary      string
	BodyHash     string
	PublishedAt  time.Time
	IngestedAt   time.Time
}

// Feed - RSS feed configuration
type Feed struct {
	Name     string
	URL      string
	Category string
	Tier     int
	Lang     string
	Active   bool
}

// MonitorSkill - main skill struct
type MonitorSkill struct {
	workspace   string
	db          *DB
	seenURLs    map[string]time.Time
	seenTitles  map[string]time.Time
	seenBodies  map[string]time.Time
	feeds       []Feed
	timeWindows map[string]time.Duration
	semaphore   chan struct{}
	mu          sync.RWMutex
}

// NewSkill creates a new MonitorSkill
func NewSkill() *MonitorSkill {
	return newSkillWithDefaults("")
}

func NewMonitorSkill(dbPath string) (*MonitorSkill, error) {
	return newSkillWithDefaults(dbPath), nil
}

func newSkillWithDefaults(dbPath string) *MonitorSkill {
	s := &MonitorSkill{
		seenURLs:   make(map[string]time.Time),
		seenTitles: make(map[string]time.Time),
		seenBodies: make(map[string]time.Time),
		semaphore:  make(chan struct{}, MaxConcurrentFetch),
		timeWindows: map[string]time.Duration{
			"breaking":   TimeWindowBreaking,
			"bangladesh": TimeWindowBD,
			"ai_labs":    TimeWindowAI,
			"china_ai":   TimeWindowAI,
			"robotics":   TimeWindowAI,
			"research":   TimeWindowResearch,
			"defence":    TimeWindowBD,
			"default":    TimeWindowBD,
		},
	}
	if dbPath != "" {
		db, err := NewDB(dbPath)
		if err == nil {
			s.db = db
			s.loadDedupCache()
		}
	}
	return s
}

// Name returns the tool name
func (s *MonitorSkill) Name() string {
	return "monitor"
}

// Description returns the tool description
func (s *MonitorSkill) Description() string {
	return `News Intelligence - Fetch, deduplicate, and rank news from multiple sources.

Commands:
- fetch: Fetch latest news from configured feeds
- status: Show monitor status and statistics
- feeds: List configured RSS feeds

Categories: breaking, bangladesh, ai_labs, china_ai, robotics, research, defence`
}

// SetWorkspace sets the workspace directory
func (s *MonitorSkill) SetWorkspace(ws string) {
	s.workspace = ws
}

// Parameters returns the tool parameters
func (s *MonitorSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command: fetch, status, or feeds",
				"enum":        []string{"fetch", "status", "feeds"},
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category to fetch: breaking, bangladesh, ai_labs, china_ai, robotics, research, defence",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max items to return",
				"default":     10,
			},
		},
		"required": []string{"command"},
	}
}

// Execute runs the monitor command
func (s *MonitorSkill) Execute(ctx context.Context, args map[string]interface{}) map[string]interface{} {
	command, _ := args["command"].(string)

	switch command {
	case "fetch":
		return s.executeFetch(ctx, args)
	case "status":
		return s.executeStatus(ctx, args)
	case "feeds":
		return s.executeFeeds(ctx, args)
	default:
		return s.errorResult(fmt.Sprintf("unknown command: %s", command))
	}
}

func (s *MonitorSkill) executeFetch(ctx context.Context, args map[string]interface{}) map[string]interface{} {
	category, _ := args["category"].(string)
	limit, _ := args["limit"].(int)
	if limit == 0 {
		limit = 10
	}

	if s.db == nil {
		db, err := NewDB(filepath.Join(s.workspace, "monitor.db"))
		if err != nil {
			return s.errorResult(fmt.Sprintf("open DB: %v", err))
		}
		s.db = db
		s.loadDedupCache()
	}

	s.loadFeeds()

	var feedsToFetch []Feed
	if category != "" {
		for _, f := range s.feeds {
			if f.Category == category && f.Active {
				feedsToFetch = append(feedsToFetch, f)
			}
		}
	} else {
		for _, f := range s.feeds {
			if f.Active {
				feedsToFetch = append(feedsToFetch, f)
			}
		}
	}

	if len(feedsToFetch) == 0 {
		return s.errorResult("no active feeds found")
	}

	var allItems []NewsItem
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, feed := range feedsToFetch {
		wg.Add(1)
		go func(f Feed) {
			defer wg.Done()
			s.semaphore <- struct{}{}
			defer func() { <-s.semaphore }()

			items, err := s.fetchFeed(f)
			mu.Lock()
			if err == nil {
				allItems = append(allItems, items...)
			}
			mu.Unlock()
		}(feed)
	}
	wg.Wait()

	var deduped []NewsItem
	for _, item := range allItems {
		if dup := s.checkDuplicate(&item); dup == nil {
			deduped = append(deduped, item)
			s.markSeen(&item)
		}
	}

	if len(deduped) > limit {
		deduped = deduped[:limit]
	}

	s.saveItems(deduped)
	s.persistDedupCache()

	return map[string]interface{}{
		"for_llm":  s.formatResults(deduped),
		"for_user": s.formatResults(deduped),
		"error":    false,
	}
}

func (s *MonitorSkill) executeStatus(ctx context.Context, args map[string]interface{}) map[string]interface{} {
	if s.db == nil {
		db, err := NewDB(filepath.Join(s.workspace, "monitor.db"))
		if err != nil {
			return s.errorResult(fmt.Sprintf("open DB: %v", err))
		}
		s.db = db
	}

	totalItems := s.db.CountItems()
	totalFeeds := 0
	for _, f := range s.feeds {
		if f.Active {
			totalFeeds++
		}
	}

	status := fmt.Sprintf(`Monitor Status:
- Active feeds: %d
- Total items: %d
- Dedup cache URLs: %d
- Dedup cache titles: %d
- Dedup cache bodies: %d`, totalFeeds, totalItems, len(s.seenURLs), len(s.seenTitles), len(s.seenBodies))

	return map[string]interface{}{
		"for_llm":  status,
		"for_user": status,
		"error":    false,
	}
}

func (s *MonitorSkill) executeFeeds(ctx context.Context, args map[string]interface{}) map[string]interface{} {
	s.loadFeeds()

	var lines []string
	lines = append(lines, "Configured Feeds:")
	for _, f := range s.feeds {
		status := "✓"
		if !f.Active {
			status = "✗"
		}
		lines = append(lines, fmt.Sprintf("  %s [%s] %s - %s (%s)", status, f.Category, f.Name, f.URL, f.Lang))
	}

	return map[string]interface{}{
		"for_llm":  strings.Join(lines, "\n"),
		"for_user": strings.Join(lines, "\n"),
		"error":    false,
	}
}

func (s *MonitorSkill) fetchFeed(feed Feed) ([]NewsItem, error) {
	parser := gofeed.NewParser()
	parser.Client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: nil,
		},
	}
	parser.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gf, err := parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", feed.URL, err)
	}

	var items []NewsItem
	for _, item := range gf.Items {
		newsItem := s.normalizeItem(item, feed)
		if newsItem != nil {
			items = append(items, *newsItem)
		}
	}

	return items, nil
}

func (s *MonitorSkill) normalizeItem(item *gofeed.Item, feed Feed) *NewsItem {
	if item.Title == "" {
		return nil
	}

	title := html.UnescapeString(item.Title)
	canonicalURL := s.canonicalizeURL(item.Link)
	bodyHash := s.hashBody(item.Description)

	return &NewsItem{
		ID:           s.generateID(canonicalURL, title),
		Source:       feed.Name,
		SourceTier:   feed.Tier,
		SourceLang:   feed.Lang,
		Category:     feed.Category,
		URL:          item.Link,
		CanonicalURL: canonicalURL,
		TitleRaw:     title,
		TitleNormal:  s.normalizeTitle(title),
		Summary:      s.cleanText(item.Description),
		BodyHash:     bodyHash,
		PublishedAt:  s.parseTime(item.PublishedParsed),
		IngestedAt:   time.Now().UTC(),
	}
}

func (s *MonitorSkill) canonicalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := parsed.Query()
	for _, param := range []string{"utm_source", "utm_medium", "utm_campaign", "ref", "source", "fbclid", "gclid"} {
		q.Del(param)
	}
	parsed.RawQuery = q.Encode()
	parsed.Fragment = ""

	return parsed.String()
}

func canonicalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := parsed.Query()
	for _, param := range []string{"utm_source", "utm_medium", "utm_campaign", "ref", "source", "fbclid", "gclid"} {
		q.Del(param)
	}
	parsed.RawQuery = q.Encode()
	parsed.Fragment = ""

	return parsed.String()
}

func (s *MonitorSkill) normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	return title
}

func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	return title
}

func (s *MonitorSkill) hashBody(text string) string {
	if text == "" {
		return ""
	}
	clean := s.cleanText(text)
	hash := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(hash[:])
}

func (s *MonitorSkill) cleanText(text string) string {
	if text == "" {
		return ""
	}
	text = html.UnescapeString(text)
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func (s *MonitorSkill) parseTime(t *time.Time) time.Time {
	if t == nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func (s *MonitorSkill) generateID(parts ...string) string {
	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

func (s *MonitorSkill) checkDuplicate(item *NewsItem) *NewsItem {
	now := time.Now()

	if dupe, ok := s.seenURLs[item.CanonicalURL]; ok && now.Sub(dupe) < 7*24*time.Hour {
		return &NewsItem{ID: "url-dup"}
	}

	if dupe, ok := s.seenBodies[item.BodyHash]; ok && now.Sub(dupe) < 7*24*time.Hour {
		return &NewsItem{ID: "body-dup"}
	}

	window := s.timeWindows[item.Category]
	if window == 0 {
		window = s.timeWindows["default"]
	}

	for normalizedTitle, seenTime := range s.seenTitles {
		if now.Sub(seenTime) > window {
			continue
		}
		score := computeSimilarityScore(item.TitleNormal, normalizedTitle)
		if score >= float32(FuzzyThreshold) {
			return &NewsItem{ID: "title-dup", TitleNormal: normalizedTitle}
		}
	}

	return nil
}

// computeSimilarityScore returns similarity score (0-100) between two titles.
//
// DECISION CHAIN:
//  1. Number-diff guard (CRITICAL): If titles differ only in numbers (e.g., "kill 12" vs "kill 20"),
//     they represent different facts and must NOT be duplicates. This runs FIRST.
//  2. TokenSortRatio: Sort words alphabetically → join → JaroWinkler. Handles word reordering.
//     If score >= 80 → duplicate.
//  3. JaroWinkler fallback: If token sort < 80, try JaroWinkler on sorted tokens.
//     If score >= 80 → duplicate.
//  4. Full similarity fallback: Original strings (not sorted) with JaroWinkler, Levenshtein, Jaccard.
//     Takes max of all three. If >= 80 → duplicate.
//
// Threshold is 80 to balance precision vs recall for news headlines.
func computeSimilarityScore(s1, s2 string) float32 {
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if hasDifferentNumbersInTitle(words1, words2) {
		return 0
	}

	sorted1 := sortWordsInTitle(words1)
	sorted2 := sortWordsInTitle(words2)
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
	levenshteinNorm := 1.0 - float32(edlib.LevenshteinDistance(s1, s2))/float32(max(len(s1), len(s2)))
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

func sortWordsInTitle(words []string) []string {
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

// CRITICAL: Must run before fuzzy scoring. Titles differing only in numbers are distinct stories
// (different death tolls, prices, counts = new facts). "Bangladesh floods kill 12" vs "kill 20"
// are NOT duplicates — they're different events with different facts.
func hasDifferentNumbersInTitle(words1, words2 []string) bool {
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

func (s *MonitorSkill) markSeen(item *NewsItem) {
	itemTime := item.PublishedAt
	if itemTime.IsZero() {
		itemTime = time.Now()
	}
	s.seenURLs[item.CanonicalURL] = itemTime
	s.seenBodies[item.BodyHash] = itemTime
	s.seenTitles[item.TitleNormal] = itemTime

	if s.db != nil {
		expireAt := itemTime.Add(7 * 24 * time.Hour)
		s.db.InsertDedupCache("url", item.CanonicalURL, itemTime, expireAt)
		s.db.InsertDedupCache("body", item.BodyHash, itemTime, expireAt)
		s.db.InsertDedupCache("title", item.TitleNormal, itemTime, expireAt)
	}
}

func (s *MonitorSkill) saveItems(items []NewsItem) {
	if len(items) == 0 || s.db == nil {
		return
	}

	for _, item := range items {
		s.db.InsertItem(item)
	}
}

func (s *MonitorSkill) persistDedupCache() {
	if s.db == nil {
		return
	}

	now := time.Now()
	for u, t := range s.seenURLs {
		s.db.InsertDedupCache("url", u, t, now.Add(7*24*time.Hour))
	}
	for t, tm := range s.seenTitles {
		s.db.InsertDedupCache("title", t, tm, now.Add(7*24*time.Hour))
	}
	for b, tm := range s.seenBodies {
		s.db.InsertDedupCache("body", b, tm, now.Add(7*24*time.Hour))
	}
}

func (s *MonitorSkill) loadDedupCache() {
	if s.db == nil {
		return
	}

	urls := s.db.GetDedupCache("url")
	for _, u := range urls {
		s.seenURLs[u.Hash] = u.SeenAt
	}

	titles := s.db.GetDedupCache("title")
	for _, t := range titles {
		s.seenTitles[t.Hash] = t.SeenAt
	}

	bodies := s.db.GetDedupCache("body")
	for _, b := range bodies {
		s.seenBodies[b.Hash] = b.SeenAt
	}
}

func (s *MonitorSkill) formatResults(items []NewsItem) string {
	if len(items) == 0 {
		return "No new items found."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Found **%d** new items:\n", len(items)))

	for i, item := range items {
		tierEmoji := "🥉"
		if item.SourceTier == 1 {
			tierEmoji = "🥇"
		} else if item.SourceTier == 2 {
			tierEmoji = "🥈"
		}

		lines = append(lines, fmt.Sprintf("%d. %s **[%s]** %s", i+1, tierEmoji, item.Source, item.TitleRaw))
		if item.Summary != "" {
			summary := item.Summary
			if len(summary) > 150 {
				summary = summary[:150] + "..."
			}
			lines = append(lines, fmt.Sprintf("   %s", summary))
		}
		lines = append(lines, fmt.Sprintf("   🔗 %s\n", item.URL))
	}

	return strings.Join(lines, "\n")
}

func (s *MonitorSkill) loadFeeds() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.feeds) > 0 {
		return
	}

	opmlPath := filepath.Join(s.workspace, "feeds.opml")
	if _, err := os.Stat(opmlPath); err == nil {
		s.feeds = s.parseOPML(opmlPath)
	}

	if len(s.feeds) == 0 {
		s.feeds = []Feed{
			{Name: "Reuters", URL: "https://feeds.reuters.com/reuters/topNews", Category: "breaking", Tier: 1, Lang: "en", Active: true},
			{Name: "bdnews24", URL: "https://bdnews24.com/rss", Category: "bangladesh", Tier: 1, Lang: "en", Active: true},
			{Name: "OpenAI", URL: "https://openai.com/news/rss.xml", Category: "ai_labs", Tier: 1, Lang: "en", Active: true},
			{Name: "DeepMind", URL: "https://deepmind.google/blog/rss.xml", Category: "ai_labs", Tier: 1, Lang: "en", Active: true},
			{Name: "HuggingFace", URL: "https://huggingface.co/blog/feed.xml", Category: "ai_labs", Tier: 1, Lang: "en", Active: true},
			{Name: "arXiv AI", URL: "https://rss.arxiv.org/rss/cs.AI", Category: "research", Tier: 1, Lang: "en", Active: true},
		}
	}
}

func (s *MonitorSkill) parseOPML(path string) []Feed {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	type OPMLHead struct {
		Title string `xml:"title"`
	}

	type OPMLOutline struct {
		XMLName  xml.Name      `xml:"outline"`
		Type     string        `xml:"type"`
		Text     string        `xml:"text"`
		Title    string        `xml:"title"`
		XMLURL   string        `xml:"xmlUrl"`
		Category string        `xml:"category"`
		Outlines []OPMLOutline `xml:"outline"`
	}

	type OPMLBody struct {
		Outlines []OPMLOutline `xml:"outline"`
	}

	type OPML struct {
		XMLName xml.Name `xml:"opml"`
		Head    OPMLHead `xml:"head"`
		Body    OPMLBody `xml:"body"`
	}

	var opml OPML
	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil
	}

	var feeds []Feed
	for _, outline := range opml.Body.Outlines {
		if outline.XMLURL != "" {
			category := outline.Category
			if category == "" {
				category = "default"
			}
			feeds = append(feeds, Feed{
				Name:     outline.Title,
				URL:      outline.XMLURL,
				Category: category,
				Tier:     2,
				Lang:     "en",
				Active:   true,
			})
		}
	}

	return feeds
}

func (s *MonitorSkill) errorResult(msg string) map[string]interface{} {
	return map[string]interface{}{
		"for_llm":  msg,
		"for_user": msg,
		"error":    true,
	}
}
