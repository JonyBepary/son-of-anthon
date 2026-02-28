package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hbollon/go-edlib"
	"github.com/jony/son-of-anthon/pkg/skills"
	"github.com/mmcdole/gofeed"
	"github.com/sipeed/picoclaw/pkg/tools"
	"golang.org/x/sync/errgroup"
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
	workspace              string
	db                     *DB
	seenURLs               map[string]time.Time
	seenTitles             map[string]time.Time
	seenBodies             map[string]time.Time
	shownURLs              map[string]int // URL -> fetch count when shown
	feeds                  []Feed
	timeWindows            map[string]time.Duration
	semaphore              chan struct{}
	mu                     sync.RWMutex
	llmProvider            LLMProvider
	recentItems            []NewsItem
	enableLLMConflictCheck bool
	maxFeedsPerCategory    int
	fetchCount             int
}

// Config holds optional configuration for MonitorSkill
type Config struct {
	DBPath                 string
	EnableLLMConflictCheck bool // Default: false (LLM conflict check disabled)
	MaxFeedsPerCategory    int  // Default: 0 (no limit)
}

type LLMProvider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error)
	GetDefaultModel() string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type LLMResponse struct {
	Content string
}

func (s *MonitorSkill) SetLLMProvider(provider LLMProvider) {
	s.llmProvider = provider
}

func (s *MonitorSkill) SetLLMConflictCheck(enabled bool) {
	s.enableLLMConflictCheck = enabled
}

func (s *MonitorSkill) IsLLMConflictCheckEnabled() bool {
	return s.enableLLMConflictCheck
}

// NewSkill creates a new MonitorSkill
func NewSkill() *MonitorSkill {
	return newSkillWithDefaults("")
}

func NewSkillWithConfig(cfg Config) *MonitorSkill {
	s := newSkillWithDefaults(cfg.DBPath)
	s.enableLLMConflictCheck = cfg.EnableLLMConflictCheck
	s.maxFeedsPerCategory = cfg.MaxFeedsPerCategory
	return s
}

func NewMonitorSkill(dbPath string) (*MonitorSkill, error) {
	return newSkillWithDefaults(dbPath), nil
}

func newSkillWithDefaults(dbPath string) *MonitorSkill {
	s := &MonitorSkill{
		seenURLs:   make(map[string]time.Time),
		seenTitles: make(map[string]time.Time),
		seenBodies: make(map[string]time.Time),
		shownURLs:  make(map[string]int),
		semaphore:  make(chan struct{}, MaxConcurrentFetch),
		fetchCount: 0,
		timeWindows: map[string]time.Duration{
			"world":      TimeWindowBreaking,
			"bangladesh": TimeWindowBD,
			"tech":       TimeWindowAI,
			"ai":         TimeWindowAI,
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
	return `News Intelligence - Fetch curated news from configured RSS feeds.

Commands:
- fetch: Fetch latest news from configured feeds (default: top 10 items)
- status: Show monitor status and statistics  
- feeds: List configured RSS feeds

Categories: world, bangladesh, tech, ai

Configure feeds in config.json under "monitor" -> "feeds`
}

// SetWorkspace sets the workspace directory
func (s *MonitorSkill) SetWorkspace(ws string) {
	s.workspace = ws
	log.Printf("[Monitor] Workspace set to: %s", ws)
	s.initWorkspace()
}

func (s *MonitorSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	os.MkdirAll(s.workspace, 0755)

	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# World Monitor - Identity

- **Name:** Pulse
- **Creature:** Globe with antenna, scanning news feeds 🌍
- **Vibe:** Cuts through noise, balanced perspective, "here's what actually matters"
- **Emoji:** 🌍
- **Catchphrase:** "Signal detected..."
`
		os.WriteFile(identityPath, []byte(identityContent), 0644)
	}

	heartbeatPath := filepath.Join(s.workspace, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatPath); os.IsNotExist(err) {
		heartbeatContent := `# HEARTBEAT.md

# Keep this file empty (or with only comments) to skip heartbeat API calls.
`
		os.WriteFile(heartbeatPath, []byte(heartbeatContent), 0644)
	}
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
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "Force fresh fetch (ignore dedup cache, get all new items)",
				"default":     false,
			},
		},
		"required": []string{"command"},
	}
}

// Execute runs the monitor command
func (s *MonitorSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "fetch":
		return s.executeFetchTool(ctx, args)
	case "status":
		return s.executeStatusTool(ctx, args)
	case "feeds":
		return s.executeFeedsTool(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("unknown command: %s", command))
	}
}

func (s *MonitorSkill) executeFetchTool(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	resultMap := s.executeFetch(ctx, args)
	content := resultMap["for_llm"].(string)

	// Write RFC cache to chief's memory dir — enables Chief morning_brief to read news
	chiefMem := filepath.Join(filepath.Dir(s.workspace), "chief", "memory")
	dateKey := time.Now().Format("20060102")
	newsPath := filepath.Join(chiefMem, "news-"+dateKey+".md")

	if items, ok := resultMap["items"].([]NewsItem); ok && len(items) > 0 {
		var rfcLines []string
		for _, item := range items {
			date := item.PublishedAt.Format("20060102")
			if date == "" || date == "00010101" {
				date = dateKey
			}
			line := skills.EncodeRecord("news", item.URL, item.TitleRaw, item.Category, date)
			rfcLines = append(rfcLines, line)
		}
		_ = skills.WriteRFCFile(newsPath, "monitor", "6h", rfcLines)
	}

	return &tools.ToolResult{
		ForLLM:  content,
		ForUser: content,
	}
}

func (s *MonitorSkill) executeStatusTool(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	result := s.executeStatus(ctx, args)
	content := result["for_llm"].(string)
	return &tools.ToolResult{
		ForLLM:  content,
		ForUser: content,
	}
}

func (s *MonitorSkill) executeFeedsTool(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	result := s.executeFeeds(ctx, args)
	content := result["for_llm"].(string)
	return &tools.ToolResult{
		ForLLM:  content,
		ForUser: content,
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

	if s.maxFeedsPerCategory > 0 && len(feedsToFetch) > s.maxFeedsPerCategory {
		feedsToFetch = feedsToFetch[:s.maxFeedsPerCategory]
	}

	if len(feedsToFetch) == 0 {
		return s.errorResult("no active feeds found")
	}

	var allItems []NewsItem
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)
	if MaxConcurrentFetch > 0 {
		g.SetLimit(MaxConcurrentFetch)
	}

	for _, feed := range feedsToFetch {
		feed := feed
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("monitor fetch panic on %s: %v", feed.URL, r)
				}
			}()

			items, fetchErr := s.fetchFeed(gCtx, feed)
			if fetchErr != nil {
				log.Printf("[Monitor] ERROR fetching feed %s (%s): %v", feed.Name, feed.URL, fetchErr)
				return fetchErr
			}

			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		log.Printf("[Monitor] Fetch encountered early cancellation: %v", err)
	}

	s.mu.Lock()
	s.fetchCount++
	currentFetch := s.fetchCount
	s.mu.Unlock()

	var deduped []NewsItem
	var rotated []NewsItem

	for _, item := range allItems {

		isNew := s.checkDuplicate(&item) == nil

		if isNew {
			if s.enableLLMConflictCheck && s.llmProvider != nil {
				if dup := s.checkLLMConflict(ctx, &item); dup != nil {
					continue
				}
			}
			deduped = append(deduped, item)
		} else {
			// Regardless of when it was shown, we can use it for rotation if needed
			// Let's rely on time windows instead of strict fetch counts for rotation
			rotated = append(rotated, item)
		}
	}

	// Shuffle to mix from different sources (not just first feed)
	if len(deduped) > 1 {
		for i := range deduped {
			j := (currentFetch + i) % len(deduped)
			deduped[i], deduped[j] = deduped[j], deduped[i]
		}
	}
	if len(rotated) > 1 {
		for i := range rotated {
			j := (currentFetch + i) % len(rotated)
			rotated[i], rotated[j] = rotated[j], rotated[i]
		}
	}

	// Implement true round-robin per-source to ensure one source doesn't dominate
	var finalDeduped []NewsItem
	itemsBySource := make(map[string][]NewsItem)
	var sources []string

	for _, item := range deduped {
		if len(itemsBySource[item.Source]) == 0 {
			sources = append(sources, item.Source)
		}
		itemsBySource[item.Source] = append(itemsBySource[item.Source], item)
	}

	for len(finalDeduped) < len(deduped) && len(sources) > 0 {
		var nextSources []string
		for _, source := range sources {
			if len(itemsBySource[source]) > 0 {
				finalDeduped = append(finalDeduped, itemsBySource[source][0])
				itemsBySource[source] = itemsBySource[source][1:]
				if len(itemsBySource[source]) > 0 {
					nextSources = append(nextSources, source)
				}
			}
		}
		sources = nextSources
	}
	deduped = finalDeduped

	s.mu.Lock()
	for _, item := range deduped {
		s.shownURLs[item.CanonicalURL] = currentFetch
		s.markSeen(&item)
		s.addToRecent(&item)
	}
	s.mu.Unlock()

	var allResults []NewsItem

	// If we don't have enough new items, aggressively use rotated items
	if len(deduped) < limit {
		allResults = append(allResults, deduped...)
		needed := limit - len(deduped)

		var eligibleRotated []NewsItem
		rotItemsBySource := make(map[string][]NewsItem)
		var rotSources []string

		for _, item := range rotated {
			s.mu.Lock()
			shownAt, wasShown := s.shownURLs[item.CanonicalURL]
			s.mu.Unlock()

			// If it wasn't shown, or was shown at least 1 fetch ago, it's eligible
			if !wasShown || (currentFetch-shownAt) >= 1 {
				if len(rotItemsBySource[item.Source]) == 0 {
					rotSources = append(rotSources, item.Source)
				}
				rotItemsBySource[item.Source] = append(rotItemsBySource[item.Source], item)
			}
		}

		// Round-robin for rotated items too
		for len(eligibleRotated) < needed && len(rotSources) > 0 {
			var nextRotSources []string
			for _, source := range rotSources {
				if len(rotItemsBySource[source]) > 0 && len(eligibleRotated) < needed {
					eligibleRotated = append(eligibleRotated, rotItemsBySource[source][0])
					rotItemsBySource[source] = rotItemsBySource[source][1:]
					if len(rotItemsBySource[source]) > 0 {
						nextRotSources = append(nextRotSources, source)
					}
				}
			}
			rotSources = nextRotSources
		}

		allResults = append(allResults, eligibleRotated...)

		s.mu.Lock()
		for _, item := range eligibleRotated {
			s.shownURLs[item.CanonicalURL] = currentFetch
		}
		s.mu.Unlock()

		// If STILL not enough, ignore maxPerSource for rotated items
		if len(allResults) < limit {
			stillNeeded := limit - len(allResults)
			var desperateRotated []NewsItem

			// Find rotated items we haven't picked yet
			for _, item := range rotated {
				picked := false
				for _, r := range allResults {
					if r.CanonicalURL == item.CanonicalURL {
						picked = true
						break
					}
				}
				if !picked {
					s.mu.Lock()
					shownAt, wasShown := s.shownURLs[item.CanonicalURL]
					s.mu.Unlock()
					if !wasShown || (currentFetch-shownAt) >= 1 {
						desperateRotated = append(desperateRotated, item)
					}
				}
			}

			if len(desperateRotated) > 0 {
				if stillNeeded > len(desperateRotated) {
					stillNeeded = len(desperateRotated)
				}
				allResults = append(allResults, desperateRotated[:stillNeeded]...)
				s.mu.Lock()
				for _, item := range desperateRotated[:stillNeeded] {
					s.shownURLs[item.CanonicalURL] = currentFetch
				}
				s.mu.Unlock()
			}
		}

	} else {
		// We have enough new items
		allResults = deduped
		if len(allResults) > limit {
			allResults = allResults[:limit]
		}
	}

	s.saveItems(allResults)
	s.persistDedupCache()

	return map[string]interface{}{
		"for_llm":  s.formatResults(allResults),
		"for_user": s.formatResults(allResults),
		"items":    allResults,
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

func (s *MonitorSkill) fetchFeed(ctx context.Context, feed Feed) ([]NewsItem, error) {
	fp := gofeed.NewParser()

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Temporarily logging feed fetch to see why they are failing
	// log.Printf("[Monitor] Fetching: %s", feed.URL)

	feedData, err := fp.ParseURLWithContext(feed.URL, reqCtx)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", feed.URL, err)
	}

	var items []NewsItem
	for _, item := range feedData.Items {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *MonitorSkill) checkLLMConflict(ctx context.Context, item *NewsItem) *NewsItem {
	if s.llmProvider == nil {
		return nil
	}

	recent := s.getRecentItems(item.Category, 10)
	if len(recent) == 0 {
		return nil
	}

	prompt := s.buildConflictPrompt(item, recent)

	resp, err := s.llmProvider.Chat(
		ctx,
		[]Message{{Role: "user", Content: prompt}},
		nil,
		s.llmProvider.GetDefaultModel(),
		map[string]interface{}{"temperature": 0.3},
	)
	if err != nil {
		return nil
	}

	if isDuplicateResponse(resp.Content) {
		return &NewsItem{ID: "llm-dup", TitleNormal: "llm-conflict"}
	}
	return nil
}

func (s *MonitorSkill) getRecentItems(category string, limit int) []NewsItem {
	var filtered []NewsItem
	for _, item := range s.recentItems {
		if item.Category == category {
			filtered = append(filtered, item)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered
}

func (s *MonitorSkill) buildConflictPrompt(item *NewsItem, recent []NewsItem) string {
	var b strings.Builder
	b.WriteString("You are a news deduplication assistant. Determine if the new article is a duplicate of any recent articles.\n\n")

	b.WriteString("Recent articles in the same category:\n")
	for i, recent := range recent {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, recent.Source, recent.TitleRaw))
		if recent.Summary != "" {
			b.WriteString(fmt.Sprintf("   Summary: %s\n", truncateString(recent.Summary, 200)))
		}
	}

	b.WriteString("\nNew article to check:\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", item.TitleRaw))
	if item.Summary != "" {
		b.WriteString(fmt.Sprintf("Summary: %s\n", truncateString(item.Summary, 200)))
	}
	b.WriteString(fmt.Sprintf("Source: %s\n", item.Source))

	b.WriteString("\nRespond with ONLY 'YES' if the new article covers the exact same event/announcement as any recent article (even if worded differently or translated into another language like Bengali vs English), or 'NO' if it's a different story.\n")
	b.WriteString("Consider: same company announcing something, same research paper, same government action, same incident etc.\n")
	b.WriteString("Answer:")

	return b.String()
}

func isDuplicateResponse(response string) bool {
	resp := strings.ToUpper(strings.TrimSpace(response))
	return strings.HasPrefix(resp, "YES")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (s *MonitorSkill) addToRecent(item *NewsItem) {
	s.recentItems = append([]NewsItem{*item}, s.recentItems...)
	if len(s.recentItems) > 100 {
		s.recentItems = s.recentItems[:100]
	}
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

	s.recentItems = s.db.GetRecentItems("", 50)
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

	// Try to load from config.json first
	configPath := filepath.Join(filepath.Dir(s.workspace), "..", "config.json")
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			var configData map[string]interface{}
			if json.Unmarshal(data, &configData) == nil {
				if monitorCfg, ok := configData["monitor"].(map[string]interface{}); ok {
					if feedsData, ok := monitorCfg["feeds"].([]interface{}); ok {
						for _, f := range feedsData {
							if feedMap, ok := f.(map[string]interface{}); ok {
								feed := Feed{
									Name:     getString(feedMap, "name", ""),
									URL:      getString(feedMap, "url", ""),
									Category: getString(feedMap, "category", "default"),
									Lang:     getString(feedMap, "lang", "en"),
									Active:   true,
								}
								if tier, ok := feedMap["tier"].(float64); ok {
									feed.Tier = int(tier)
								} else {
									feed.Tier = 1
								}
								if active, ok := feedMap["active"].(bool); ok {
									feed.Active = active
								}
								if feed.URL != "" {
									s.feeds = append(s.feeds, feed)
								}
							}
						}
						log.Printf("[Monitor] Loaded %d feeds from config.json", len(s.feeds))
					}
				}
			}
		}
	}

	// Fall back to OPML if no feeds from config
	if len(s.feeds) == 0 {
		opmlPath := filepath.Join(s.workspace, "feeds.opml")
		log.Printf("[Monitor] Loading feeds from: %s", opmlPath)
		if _, err := os.Stat(opmlPath); err == nil {
			s.feeds = s.parseOPML(opmlPath)
			log.Printf("[Monitor] Loaded %d feeds from OPML", len(s.feeds))
		}
	}

	// Final fallback to defaults
	if len(s.feeds) == 0 {
		s.feeds = []Feed{
			{Name: "Reuters", URL: "https://feeds.reuters.com/reuters/topNews", Category: "world", Tier: 1, Lang: "en", Active: true},
			{Name: "BBC", URL: "http://feeds.bbci.co.uk/news/world/rss.xml", Category: "world", Tier: 1, Lang: "en", Active: true},
			{Name: "bdnews24", URL: "https://bdnews24.com/rss", Category: "bangladesh", Tier: 1, Lang: "en", Active: true},
			{Name: "The Daily Star", URL: "https://www.thedailystar.net/rss.xml", Category: "bangladesh", Tier: 1, Lang: "en", Active: true},
			{Name: "OpenAI", URL: "https://openai.com/news/rss.xml", Category: "tech", Tier: 1, Lang: "en", Active: true},
			{Name: "TechCrunch", URL: "https://techcrunch.com/feed/", Category: "tech", Tier: 1, Lang: "en", Active: true},
			{Name: "Hacker News", URL: "https://hnrss.org/frontpage", Category: "tech", Tier: 1, Lang: "en", Active: true},
			{Name: "arXiv AI", URL: "https://rss.arxiv.org/rss/cs.AI", Category: "ai", Tier: 1, Lang: "en", Active: true},
		}
		log.Printf("[Monitor] Using default feeds")
	}
}

func getString(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

type opmlOutline struct {
	XMLName  xml.Name      `xml:"outline"`
	Type     string        `xml:"type,attr"`
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Category string        `xml:"category,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlDoc struct {
	XMLName xml.Name `xml:"opml"`
	Body    opmlBody `xml:"body"`
}

func (s *MonitorSkill) parseOPML(path string) []Feed {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var opml opmlDoc
	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil
	}

	var feeds []Feed
	s.parseOPMLOutlines(opml.Body.Outlines, "", &feeds)
	return feeds
}

func (s *MonitorSkill) parseOPMLOutlines(outlines []opmlOutline, parentCategory string, feeds *[]Feed) {
	for _, outline := range outlines {
		category := s.mapCategory(outline.Text, outline.Title, parentCategory)

		if outline.XMLURL != "" {
			name := outline.Title
			if name == "" {
				name = outline.Text
			}
			*feeds = append(*feeds, Feed{
				Name:     name,
				URL:      outline.XMLURL,
				Category: category,
				Tier:     2,
				Lang:     "en",
				Active:   true,
			})
		}

		if len(outline.Outlines) > 0 {
			s.parseOPMLOutlines(outline.Outlines, category, feeds)
		}
	}
}

func (s *MonitorSkill) mapCategory(text, title, parent string) string {
	lowerText := strings.ToLower(text)
	lowerTitle := strings.ToLower(title)
	combined := lowerText + " " + lowerTitle
	lowerParent := strings.ToLower(parent)

	if strings.Contains(combined, "bangladesh") || strings.Contains(combined, " bd ") || strings.Contains(lowerParent, "bangladesh") {
		return "bangladesh"
	}
	if strings.Contains(combined, "breaking") || strings.Contains(combined, "wire") || strings.Contains(combined, "reuters") || strings.Contains(combined, "ap ") || strings.Contains(combined, "bbc") {
		return "breaking"
	}
	if strings.Contains(combined, "ai") || strings.Contains(combined, "llm") || strings.Contains(combined, "model") || strings.Contains(combined, "gpt") || strings.Contains(combined, "gemini") || strings.Contains(combined, "claude") {
		return "ai_labs"
	}
	if strings.Contains(combined, "china") || strings.Contains(combined, "chinese") {
		return "china_ai"
	}
	if strings.Contains(combined, "robot") || strings.Contains(combined, "humanoid") || strings.Contains(combined, "drone") || strings.Contains(combined, "autonomous vehicle") {
		return "robotics"
	}
	if strings.Contains(combined, "defence") || strings.Contains(combined, "defense") || strings.Contains(combined, "military") || strings.Contains(combined, "security") {
		return "defence"
	}
	if strings.Contains(combined, "research") || strings.Contains(combined, "arxiv") || strings.Contains(combined, "academic") || strings.Contains(combined, "paper") {
		return "research"
	}

	if parent != "" {
		return s.mapCategory(parent, "", "") // Recurse to map parent category
	}

	return "default"
}

func (s *MonitorSkill) errorResult(msg string) map[string]interface{} {
	return map[string]interface{}{
		"for_llm":  msg,
		"for_user": msg,
		"error":    true,
	}
}
