package research

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mtreilly/goarxiv"
	"github.com/sipeed/picoclaw/pkg/tools"
)

const (
	huggingFacePapersURL = "https://huggingface.co/papers"
	arxivAPIURL          = "http://arxiv.org/api/query"
	maxFileSize          = 50 * 1024 * 1024 // 50MB
)

type Paper struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	ArxivID       string `json:"arxiv_id,omitempty"`
	Source        string `json:"source"`
	CoreRank      string `json:"core_rank,omitempty"`
	PublishedDate string `json:"published_date,omitempty"`
	Abstract      string `json:"abstract,omitempty"`
}

type FetchResult struct {
	Papers     []Paper `json:"papers"`
	TotalFound int     `json:"total_found"`
	Query      string  `json:"query"`
	Timestamp  string  `json:"timestamp"`
	Error      string  `json:"error,omitempty"`
}

type DownloadResult struct {
	Status   string `json:"status"` // success, error, link_only
	FilePath string `json:"file_path,omitempty"`
	Filename string `json:"filename,omitempty"`
	URL      string `json:"url,omitempty"`
	Message  string `json:"message,omitempty"`
}

type CoreRanking struct {
	rankings map[string]string
}

func NewCoreRanking() *CoreRanking {
	return &CoreRanking{
		rankings: make(map[string]string),
	}
}

func (c *CoreRanking) LoadFromCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		// Format: ID|Rank|ShortName|FullName
		if len(record) > 3 {
			rank := record[1]
			shortName := record[2]
			if shortName != "" {
				c.rankings[strings.ToLower(shortName)] = rank
			}
			if len(record) > 4 {
				c.rankings[strings.ToLower(record[3])] = rank
			}
		}
	}
	return nil
}

func (c *CoreRanking) GetRank(venueName string) string {
	if venueName == "" {
		return "Unranked"
	}
	if rank, ok := c.rankings[strings.ToLower(venueName)]; ok {
		return rank
	}
	return "Unranked"
}

type ResearchSkill struct {
	workspace string
	core      *CoreRanking
}

func NewSkill() *ResearchSkill {
	return &ResearchSkill{
		core: NewCoreRanking(),
	}
}

func (s *ResearchSkill) Name() string {
	return "research"
}

func (s *ResearchSkill) Description() string {
	return `Research Scout - Discover trending papers from HuggingFace and ArXiv with CORE ranking.

Use this tool to:
1. Find papers on any topic (e.g., "Find papers on LLM optimization")
2. Discover SOTA for a task (e.g., "What's SOTA for object detection?")
3. Get daily/weekly trending papers
4. Download specific papers

Commands:
- fetch: Search for papers by topic
- download: Download a specific paper by ID
- memory: Check what papers were found previously`
}

func (s *ResearchSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute: fetch, download, or memory",
				"enum":        []string{"fetch", "download", "memory"},
			},
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Topic to search for (for fetch command)",
			},
			"timeframe": map[string]interface{}{
				"type":        "string",
				"description": "Timeframe for trending papers: daily, weekly, monthly, search",
				"enum":        []string{"daily", "weekly", "monthly", "search"},
				"default":     "daily",
			},
			"include_arxiv": map[string]interface{}{
				"type":        "boolean",
				"description": "Also fetch from ArXiv API as supplement",
				"default":     false,
			},
			"paper_id": map[string]interface{}{
				"type":        "string",
				"description": "Paper ID to download (for download command)",
			},
			"paper_title": map[string]interface{}{
				"type":        "string",
				"description": "Paper title (for download command)",
			},
			"paper_url": map[string]interface{}{
				"type":        "string",
				"description": "Paper URL (for download command)",
			},
		},
		"required": []string{"command"},
	}
}

func (s *ResearchSkill) SetWorkspace(workspace string) {
	s.workspace = workspace
}

func (s *ResearchSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "fetch":
		return s.executeFetch(ctx, args)
	case "download":
		return s.executeDownload(ctx, args)
	case "memory":
		return s.executeMemory(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

func (s *ResearchSkill) executeFetch(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	topic, _ := args["topic"].(string)
	timeframe, _ := args["timeframe"].(string)
	if timeframe == "" {
		timeframe = "daily"
	}
	includeArxiv, _ := args["include_arxiv"].(bool)

	// Use ArXiv by default for better abstracts, optionally add HuggingFace
	var papers []Paper

	// Fetch from ArXiv (primary - gives abstracts)
	arxivPapers := s.fetchArxiv(topic, 10)
	papers = append(papers, arxivPapers...)

	// Optionally add HuggingFace (for trending)
	if includeArxiv {
		hfPapers := s.fetchHuggingFace(topic, timeframe)
		// Merge, avoiding duplicates
		seen := make(map[string]bool)
		for _, p := range papers {
			seen[p.ArxivID] = true
		}
		for _, p := range hfPapers {
			if !seen[p.ArxivID] {
				papers = append(papers, p)
			}
		}
	}

	// Assign IDs and ranks
	for i := range papers {
		if papers[i].ID == "" {
			papers[i].ID = strconv.Itoa(i + 1)
		}
		if papers[i].CoreRank == "" {
			papers[i].CoreRank = s.core.GetRank("arxiv")
		}
	}

	// Save to memory
	s.saveToMemory(papers, topic)

	result := FetchResult{
		Papers:     papers,
		TotalFound: len(papers),
		Query:      topic,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	if len(papers) == 0 {
		result.Error = "No papers found"
	}

	jsonData, _ := json.MarshalIndent(result, "", "  ")
	return &tools.ToolResult{
		ForLLM:  string(jsonData),
		ForUser: formatPapersForUser(papers),
		Silent:  false,
		IsError: false,
	}
}

func (s *ResearchSkill) executeDownload(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	paperID, _ := args["paper_id"].(string)
	paperTitle, _ := args["paper_title"].(string)
	paperURL, _ := args["paper_url"].(string)

	if paperURL == "" {
		return tools.ErrorResult("paper_url is required")
	}

	// Extract ID from URL if paperID is empty
	if paperID == "" {
		re := regexp.MustCompile(`(\d+\.\d+)`)
		if m := re.FindStringSubmatch(paperURL); len(m) > 1 {
			paperID = m[1]
		}
	}

	// Get PDF URL
	pdfURL := strings.Replace(paperURL, "/abs/", "/pdf/", 1)

	// Check file size
	if size, err := s.checkFileSize(pdfURL); err == nil && size > maxFileSize {
		result := DownloadResult{
			Status:  "error",
			Message: fmt.Sprintf("File too large (%.1fMB). Limit is 50MB.", float64(size)/1024/1024),
			URL:     pdfURL,
		}
		jsonData, _ := json.Marshal(result)
		return &tools.ToolResult{
			ForLLM:  string(jsonData),
			ForUser: result.Message,
			Silent:  false,
			IsError: false,
		}
	}

	// Download
	// Extract ID from URL if paperTitle is empty
	if paperTitle == "" && paperURL != "" {
		// Extract arxiv ID from URL like http://arxiv.org/abs/2402.12251v2
		re := regexp.MustCompile(`(\d+\.\d+)`)
		if m := re.FindStringSubmatch(paperURL); len(m) > 1 {
			paperTitle = m[1]
		}
	}

	// Generate filename
	var filename string
	if paperTitle != "" {
		filename = fmt.Sprintf("%s_%s.pdf", paperID, sanitizeFilename(paperTitle))
	} else {
		filename = fmt.Sprintf("%s.pdf", paperID)
	}
	filepath := filepath.Join(s.workspace, filename)

	if err := s.downloadFile(pdfURL, filepath); err != nil {
		result := DownloadResult{
			Status:  "link_only",
			Message: "Download failed. Here's the direct link:",
			URL:     pdfURL,
		}
		jsonData, _ := json.Marshal(result)
		return &tools.ToolResult{
			ForLLM:  string(jsonData),
			ForUser: fmt.Sprintf("%s\n%s", result.Message, pdfURL),
			Silent:  false,
			IsError: false,
		}
	}

	result := DownloadResult{
		Status:   "success",
		FilePath: filepath,
		Filename: filename,
	}
	jsonData, _ := json.Marshal(result)
	return &tools.ToolResult{
		ForLLM:  string(jsonData),
		ForUser: fmt.Sprintf("Downloaded: %s", filename),
		Silent:  false,
		IsError: false,
	}
}

func (s *ResearchSkill) executeMemory(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	memoryPath := filepath.Join(s.workspace, "memory", "research-papers.md")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return &tools.ToolResult{
			ForLLM:  "No memory found",
			ForUser: "No research papers in memory yet.",
			Silent:  false,
			IsError: false,
		}
	}
	return &tools.ToolResult{
		ForLLM:  string(data),
		ForUser: string(data),
		Silent:  false,
		IsError: false,
	}
}

func (s *ResearchSkill) fetchHuggingFace(topic, timeframe string) []Paper {
	var url string
	today := time.Now().Format("2006-01-02")

	switch timeframe {
	case "daily":
		url = fmt.Sprintf("%s/date/%s", huggingFacePapersURL, today)
		if topic != "" {
			url += "?q=" + strings.ReplaceAll(topic, " ", "+")
		}
	case "weekly":
		year, week := time.Now().ISOWeek()
		url = fmt.Sprintf("%s/week/%d-W%02d", huggingFacePapersURL, year, week)
		if topic != "" {
			url += "?q=" + strings.ReplaceAll(topic, " ", "+")
		}
	case "monthly":
		url = fmt.Sprintf("%s/month/%s", huggingFacePapersURL, time.Now().Format("2006-01"))
		if topic != "" {
			url += "?q=" + strings.ReplaceAll(topic, " ", "+")
		}
	default: // search
		if topic != "" {
			url = fmt.Sprintf("%s?q=%s", huggingFacePapersURL, strings.ReplaceAll(topic, " ", "+"))
		} else {
			url = huggingFacePapersURL
		}
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ResearchScout/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return s.parseHuggingFaceHTML(string(body))
}

func (s *ResearchSkill) parseHuggingFaceHTML(html string) []Paper {
	var papers []Paper

	// HuggingFace papers page has paper IDs in /papers/ARXIV_ID format
	// Get all unique paper IDs
	idRe := regexp.MustCompile(`href="/papers/([0-9]+\.[0-9]+)"`)
	idMatches := idRe.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var paperIDs []string
	for _, m := range idMatches {
		if len(m) > 1 {
			id := m[1]
			if !seen[id] {
				seen[id] = true
				paperIDs = append(paperIDs, id)
			}
		}
	}

	// Get titles from various patterns
	titleRe := regexp.MustCompile(`<h3[^>]*>([^<]+)</h3>`)
	titleMatches := titleRe.FindAllStringSubmatch(html, -1)

	// Get dates
	dateRe := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	dateMatches := dateRe.FindAllString(html, -1)

	// Get abstracts - try multiple patterns
	abstractPatterns := []string{
		`class="text-gray-500[^"]*"[^>]*>([^<]+)</p>`,
		`class="line-clamp-3[^"]*"[^>]*>([^<]+)</p>`,
		`<p[^>]*>([^<]{50,200})</p>`,
	}
	var abstracts []string
	for _, pattern := range abstractPatterns {
		absRe := regexp.MustCompile(pattern)
		abs := absRe.FindAllStringSubmatch(html, -1)
		for _, a := range abs {
			if len(a) > 1 && len(a[1]) > 30 {
				abstracts = append(abstracts, a[1])
			}
		}
		if len(abstracts) >= len(paperIDs) {
			break
		}
	}

	// Build papers list
	for i, arxivID := range paperIDs {
		if i >= 10 {
			break
		}

		paper := Paper{
			Title:         fmt.Sprintf("Paper %s", arxivID),
			URL:           fmt.Sprintf("https://arxiv.org/abs/%s", arxivID),
			ArxivID:       arxivID,
			Source:        "huggingface",
			PublishedDate: "Unknown",
		}

		// Try to get title
		if i < len(titleMatches) && len(titleMatches[i]) > 1 {
			title := strings.TrimSpace(titleMatches[i][1])
			title = strings.ReplaceAll(title, "<span class=\"highlight\">", "")
			title = strings.ReplaceAll(title, "</span>", "")
			if len(title) > 5 {
				paper.Title = title
			}
		}

		// Try to get date
		if i < len(dateMatches) {
			paper.PublishedDate = dateMatches[i]
		}

		// Try to get abstract
		if i < len(abstracts) {
			paper.Abstract = strings.TrimSpace(abstracts[i])
		}

		papers = append(papers, paper)
	}

	// If we got papers but no abstracts, fetch abstracts from ArXiv for each
	if len(papers) > 0 && papers[0].Abstract == "" {
		// Fetch from ArXiv to get abstracts
		if len(paperIDs) > 0 {
			arxivPapers := s.fetchArxivByIDs(paperIDs[:min(5, len(paperIDs))])
			for i, ap := range arxivPapers {
				if i < len(papers) {
					papers[i].Abstract = ap.Abstract
					papers[i].Title = ap.Title
					papers[i].PublishedDate = ap.PublishedDate
				}
			}
		}
	}

	return papers
}

func (s *ResearchSkill) fetchArxiv(topic string, maxResults int) []Paper {
	client, err := goarxiv.New()
	if err != nil {
		return nil
	}

	ctx := context.Background()
	results, err := client.Search(ctx, fmt.Sprintf("all:%s", topic), &goarxiv.SearchOptions{
		MaxResults: maxResults,
	})
	if err != nil {
		return nil
	}

	var papers []Paper
	for _, article := range results.Articles {
		arxivID := article.BaseID()
		papers = append(papers, Paper{
			Title:         article.Title,
			URL:           article.ID,
			ArxivID:       arxivID,
			Source:        "arxiv",
			PublishedDate: article.Published.Format("2006-01-02"),
			Abstract:      article.Summary,
		})
	}

	return papers
}

func (s *ResearchSkill) parseArxivXML(xml string) []Paper {
	var papers []Paper

	entryRe := regexp.MustCompile(`<entry>(.*?)</entry>`)
	titleRe := regexp.MustCompile(`<title>([^<]+)</title>`)
	summaryRe := regexp.MustCompile(`<summary>([^<]+)</summary>`)
	dateRe := regexp.MustCompile(`<published>([^<]+)</published>`)
	idRe := regexp.MustCompile(`<id>([^<]+)</id>`)

	entries := entryRe.FindAllStringSubmatch(xml, -1)

	for i, entry := range entries {
		if i >= 5 {
			break
		}
		content := entry[1]

		titleMatch := titleRe.FindStringSubmatch(content)
		summaryMatch := summaryRe.FindStringSubmatch(content)
		dateMatch := dateRe.FindStringSubmatch(content)
		idMatch := idRe.FindStringSubmatch(content)

		if len(titleMatch) > 1 {
			paper := Paper{
				Title:         strings.TrimSpace(titleMatch[1]),
				Source:        "arxiv",
				PublishedDate: "Unknown",
			}

			if len(idMatch) > 1 {
				id := idMatch[1]
				paper.URL = id
				re := regexp.MustCompile(`(\d+\.\d+)`)
				if m := re.FindStringSubmatch(id); len(m) > 1 {
					paper.ArxivID = m[1]
				}
			}

			if len(summaryMatch) > 1 {
				paper.Abstract = strings.TrimSpace(summaryMatch[1])
				if len(paper.Abstract) > 500 {
					paper.Abstract = paper.Abstract[:500]
				}
			}

			if len(dateMatch) > 1 {
				paper.PublishedDate = dateMatch[1][:10]
			}

			papers = append(papers, paper)
		}
	}

	return papers
}

func (s *ResearchSkill) fetchArxivByIDs(ids []string) []Paper {
	if len(ids) == 0 {
		return nil
	}

	client, err := goarxiv.New()
	if err != nil {
		return nil
	}

	ctx := context.Background()
	var papers []Paper

	for _, id := range ids {
		results, err := client.Search(ctx, fmt.Sprintf("id:%s", id), &goarxiv.SearchOptions{
			MaxResults: 1,
		})
		if err != nil || len(results.Articles) == 0 {
			continue
		}

		article := results.Articles[0]
		arxivID := article.BaseID()
		papers = append(papers, Paper{
			Title:         article.Title,
			URL:           article.ID,
			ArxivID:       arxivID,
			Source:        "arxiv",
			PublishedDate: article.Published.Format("2006-01-02"),
			Abstract:      article.Summary,
		})
	}

	return papers
}

func (s *ResearchSkill) saveToMemory(papers []Paper, query string) {
	if len(papers) == 0 {
		return
	}

	memoryDir := filepath.Join(s.workspace, "memory")
	os.MkdirAll(memoryDir, 0755)

	memoryPath := filepath.Join(memoryDir, "research-papers.md")
	f, err := os.OpenFile(memoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Write header if new file
	if stat, _ := f.Stat(); stat.Size() == 0 {
		f.WriteString("# Research Scout Findings\n\n")
	}

	f.WriteString(fmt.Sprintf("## Query: %s (%s)\n\n", query, time.Now().Format("2006-01-02 15:04")))

	for _, p := range papers {
		f.WriteString(fmt.Sprintf("### %s\n", p.Title))
		f.WriteString(fmt.Sprintf("- **ID**: %s\n", p.ID))
		f.WriteString(fmt.Sprintf("- **Source**: %s\n", p.Source))
		f.WriteString(fmt.Sprintf("- **Rank**: %s\n", p.CoreRank))
		f.WriteString(fmt.Sprintf("- **URL**: %s\n", p.URL))
		f.WriteString(fmt.Sprintf("- **Date**: %s\n", p.PublishedDate))
		if p.Abstract != "" {
			abstract := p.Abstract
			if len(abstract) > 200 {
				abstract = abstract[:200] + "..."
			}
			f.WriteString(fmt.Sprintf("- **Abstract**: %s\n", abstract))
		}
		f.WriteString("\n")
	}
}

func (s *ResearchSkill) checkFileSize(url string) (int64, error) {
	resp, err := http.Head(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.ContentLength, nil
}

func (s *ResearchSkill) downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func formatPapersForUser(papers []Paper) string {
	if len(papers) == 0 {
		return "No papers found."
	}

	var sb strings.Builder
	sb.WriteString("Found **")
	sb.WriteString(strconv.Itoa(len(papers)))
	sb.WriteString("** papers:\n\n")

	maxShow := 5
	if len(papers) < maxShow {
		maxShow = len(papers)
	}

	for i := 0; i < maxShow; i++ {
		p := papers[i]
		sb.WriteString(fmt.Sprintf("%d. **%s** (Rank: %s | Source: %s | Date: %s)\n",
			i+1, p.Title, p.CoreRank, p.Source, p.PublishedDate))
		if p.Abstract != "" {
			sb.WriteString(fmt.Sprintf("   Abstract: %s\n", p.Abstract))
		}
		sb.WriteString(fmt.Sprintf("   📄 %s\n\n", p.URL))
	}

	sb.WriteString("---JSON FOR RANKING---\n")
	sb.WriteString("```json\n")
	// Return structured data for model to rank
	type RankPaper struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Abstract  string `json:"abstract"`
		URL       string `json:"url"`
		Source    string `json:"source"`
		CoreRank  string `json:"core_rank"`
		Published string `json:"published_date"`
	}
	rankPapers := make([]RankPaper, len(papers))
	for i, p := range papers {
		rankPapers[i] = RankPaper{
			ID:        strconv.Itoa(i + 1),
			Title:     p.Title,
			Abstract:  p.Abstract,
			URL:       p.URL,
			Source:    p.Source,
			CoreRank:  p.CoreRank,
			Published: p.PublishedDate,
		}
	}
	rankJSON, _ := json.MarshalIndent(rankPapers, "", "  ")
	sb.WriteString(string(rankJSON))
	sb.WriteString("\n```\n")

	sb.WriteString("\nWould you like me to download any of these?")
	return sb.String()
}

func sanitizeFilename(name string) string {
	// Remove/replace unsafe characters but keep dots (for arxiv IDs)
	name = regexp.MustCompile(`[^\w\s\.-]`).ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, " ", "_")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}
