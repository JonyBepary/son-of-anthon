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

	"github.com/jony/son-of-anthon/pkg/skills"
	"github.com/mtreilly/goarxiv"
	"github.com/sipeed/picoclaw/pkg/tools"
	"golang.org/x/net/html"
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
	s.initWorkspace()
}

func (s *ResearchSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	os.MkdirAll(s.workspace, 0755)

	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Research Scout - Identity

- **Name:** Scout
- **Creature:** Academic owl with reading glasses 🦉
- **Vibe:** Nerdy enthusiasm, citation-obsessed, "did you see this paper?!"
- **Emoji:** 🔬
- **Catchphrase:** "Found something fascinating..."
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

	// Primary source: HuggingFace (trending papers)
	var papers []Paper

	hfPapers := s.fetchHuggingFace(topic, timeframe)
	papers = append(papers, hfPapers...)

	// Optionally add ArXiv (as supplement)
	if includeArxiv {
		arxivPapers := s.fetchArxiv(topic, 10)
		// Merge, avoiding duplicates
		seen := make(map[string]bool)
		for _, p := range papers {
			if p.ArxivID != "" {
				seen[p.ArxivID] = true
			}
		}
		for _, p := range arxivPapers {
			id := p.ArxivID
			if id == "" {
				id = p.Title
			}
			if !seen[id] {
				papers = append(papers, p)
				seen[id] = true
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
		safeTitle := sanitizeFilename(paperTitle)
		filename = fmt.Sprintf("%s_%s.pdf", paperID, safeTitle)
	} else {
		filename = fmt.Sprintf("%s.pdf", paperID)
	}

	// Double check path traversal protection
	filename = filepath.Base(filepath.Clean("/" + filename))
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

func (s *ResearchSkill) parseHuggingFaceHTML(htmlContent string) []Paper {
	var papers []Paper

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return papers
	}

	seenIDs := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// Stop if we hit 10 papers
		if len(papers) >= 10 {
			return
		}

		if n.Type == html.ElementNode && n.Data == "div" {
			// Find paper cards
			hasFlexCol := false
			hasJustifyBetween := false
			for _, attr := range n.Attr {
				if attr.Key == "class" {
					if strings.Contains(attr.Val, "flex-col") {
						hasFlexCol = true
					}
					if strings.Contains(attr.Val, "justify-between") {
						hasJustifyBetween = true
					}
				}
			}

			if hasFlexCol && hasJustifyBetween {
				// We found a paper card. Extract data.
				paper := s.extractPaperFromCard(n)
				if paper != nil && !seenIDs[paper.ArxivID] {
					seenIDs[paper.ArxivID] = true
					papers = append(papers, *paper)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// If we got papers but no abstracts, fetch abstracts from ArXiv for each
	if len(papers) > 0 && papers[0].Abstract == "" {
		var paperIDs []string
		for _, p := range papers {
			paperIDs = append(paperIDs, p.ArxivID)
		}

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

func (s *ResearchSkill) extractPaperFromCard(cardNode *html.Node) *Paper {
	var title, arxivID, abstract, pubDate string
	var paperURL string

	// Extract Title and ID
	var findTitle func(*html.Node)
	findTitle = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "h3" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "a" {
					for _, attr := range c.Attr {
						if attr.Key == "href" && strings.Contains(attr.Val, "/papers/") {
							parts := strings.Split(attr.Val, "/")
							if len(parts) > 0 {
								potentialID := parts[len(parts)-1]
								if matched, _ := regexp.MatchString(`^\d{4}\.\d{4,5}$`, potentialID); matched {
									arxivID = potentialID
									paperURL = fmt.Sprintf("https://arxiv.org/abs/%s", arxivID)
								}
							}
							title = s.extractText(c)
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findTitle(c)
		}
	}
	findTitle(cardNode)

	if arxivID == "" || title == "" || len(title) < 10 {
		return nil
	}

	// Extract Abstract
	var findAbstract func(*html.Node)
	findAbstract = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "text-gray-500") {
					abstract = s.extractText(n)
					if len(abstract) > 500 {
						abstract = abstract[:500]
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findAbstract(c)
		}
	}
	findAbstract(cardNode)

	// Extract Date
	var findDate func(*html.Node)
	findDate = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "date" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "text-gray-350") {
					pubDate = s.extractText(n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findDate(c)
		}
	}
	findDate(cardNode)

	if pubDate == "" {
		pubDate = "Unknown"
	}

	return &Paper{
		Title:         title,
		URL:           paperURL,
		ArxivID:       arxivID,
		Source:        "huggingface",
		PublishedDate: pubDate,
		Abstract:      abstract,
	}
}

func (s *ResearchSkill) extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		// Clean up spacing and whitespace
		return strings.TrimSpace(n.Data)
	}

	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		childText := s.extractText(c)
		if childText != "" {
			if text.Len() > 0 {
				text.WriteString(" ")
			}
			text.WriteString(childText)
		}
	}
	// Final clean of internal double spaces that might occur
	return strings.Join(strings.Fields(text.String()), " ")
}

func (s *ResearchSkill) fetchArxiv(topic string, maxResults int) []Paper {
	client, err := goarxiv.New()
	if err != nil {
		return nil
	}

	// Format query to enforce phrase matching if it contains spaces
	query := topic
	if strings.Contains(query, " ") && !strings.HasPrefix(query, "\"") {
		query = fmt.Sprintf("\"%s\"", query)
	}

	ctx := context.Background()
	results, err := client.Search(ctx, fmt.Sprintf("all:%s", query), &goarxiv.SearchOptions{
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

	// Derive chief memory dir from research workspace
	chiefMem := filepath.Join(filepath.Dir(s.workspace), "chief", "memory")
	dateKey := time.Now().Format("20060102")
	researchPath := filepath.Join(chiefMem, "research-"+dateKey+".md")

	var rfcLines []string
	for _, p := range papers {
		date := strings.ReplaceAll(p.PublishedDate, "-", "")
		if len(date) > 8 {
			date = date[:8]
		}
		if date == "" {
			date = dateKey
		}
		line := skills.EncodeRecord("paper", p.URL, p.Title, query, date)
		rfcLines = append(rfcLines, line)
	}
	_ = skills.WriteRFCFile(researchPath, "research", "24h", rfcLines)
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
