package atc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
)

// ATCCalendarConfig holds the Nextcloud CalDAV credentials for ATC sync operations.
type ATCCalendarConfig struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout_seconds"`
}

// loadATCConfig parses the config file for Nextcloud calendar settings.
func loadATCConfig() ATCCalendarConfig {
	var cfg struct {
		Tools struct {
			Nextcloud ATCCalendarConfig `json:"nextcloud"`
		} `json:"tools"`
	}
	home, _ := os.UserHomeDir()
	path := os.Getenv("PERSONAL_OS_CONFIG")
	if path == "" {
		path = filepath.Join(home, ".picoclaw", "config.json")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &cfg)
	}
	return cfg.Tools.Nextcloud
}

// TaskOptions holds optional metadata for a CalDAV VTODO
type TaskOptions struct {
	Due             string // RFC3339 datetime, e.g. 2026-02-21T17:00:00Z
	Start           string // RFC3339 datetime for DTSTART
	Priority        int    // 1=High, 5=Medium, 9=Low, 0=None (RFC 5545)
	PercentComplete int    // 0-100
	Location        string
	URL             string
	Notes           string // DESCRIPTION field
}

func buildTasksURL(cfg ATCCalendarConfig) string {
	return caldav.BuildTasksURL(cfg.Host, cfg.Username)
}

// pushTaskToCalDAV creates or updates a VTODO on the Nextcloud CalDAV server via HTTP PUT.
func pushTaskToCalDAV(cfg ATCCalendarConfig, taskUID, summary string, opts TaskOptions) error {
	base := buildTasksURL(cfg)
	putURL := base + taskUID + ".ics"

	// Build VTODO fields conditionally
	var extra string
	if opts.Start != "" {
		extra += "DTSTART:" + formatRFC3339ToICS(opts.Start) + "\r\n"
	}
	if opts.Due != "" {
		extra += "DUE:" + formatRFC3339ToICS(opts.Due) + "\r\n"
	}
	if opts.Priority > 0 {
		extra += fmt.Sprintf("PRIORITY:%d\r\n", opts.Priority)
	}
	if opts.PercentComplete > 0 {
		extra += fmt.Sprintf("PERCENT-COMPLETE:%d\r\n", opts.PercentComplete)
	}
	if opts.Location != "" {
		extra += "LOCATION:" + opts.Location + "\r\n"
	}
	if opts.URL != "" {
		extra += "URL:" + opts.URL + "\r\n"
	}
	if opts.Notes != "" {
		extra += "DESCRIPTION:" + strings.ReplaceAll(opts.Notes, "\n", "\\n") + "\r\n"
	}

	icsBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Son of Anthon ATC//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:" + taskUID + "\r\n" +
		"SUMMARY:" + summary + "\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		extra +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"

	req, err := http.NewRequest(http.MethodPut, putURL, strings.NewReader(icsBody))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if cfg.Username != "" && cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP PUT failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("CalDAV returned status %d for PUT", resp.StatusCode)
	}
	return nil
}

// formatRFC3339ToICS delegates to the shared caldav package.
func formatRFC3339ToICS(ts string) string {
	return caldav.FormatRFC3339ToICS(ts)
}

// listNextcloudTasks does a CalDAV PROPFIND to return all task filenames (UIDs) in the tasks/ collection.
func listNextcloudTasks(cfg ATCCalendarConfig) ([]string, error) {
	base := buildTasksURL(cfg)
	if cfg.Host == "" || cfg.Username == "" {
		return nil, fmt.Errorf("host and username not configured in config.json")
	}
	req, err := http.NewRequest("PROPFIND", base, strings.NewReader(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PROPFIND failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading PROPFIND response: %w", err)
	}

	// Extract .ics hrefs — Nextcloud uses lowercase 'd:href', so match case-insensitively
	var hrefs []string
	for _, line := range strings.Split(string(body), "<") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "d:href>") || strings.HasPrefix(lower, "href>") {
			val := strings.SplitN(line, ">", 2)
			if len(val) == 2 && strings.HasSuffix(strings.TrimSpace(val[1]), ".ics") {
				hrefs = append(hrefs, strings.TrimSpace(val[1]))
			}
		}
	}
	return hrefs, nil
}

// deleteTaskFromCalDAV sends an HTTP DELETE for the given CalDAV href path.
func deleteTaskFromCalDAV(cfg ATCCalendarConfig, href string) error {
	// href is a path like /remote.php/dav/calendars/user/tasks/uid.ics
	// Build the full URL from the base host
	tasksURL := buildTasksURL(cfg)
	idx := strings.Index(tasksURL, "/remote.php")
	var fullURL string
	if idx > 0 && !strings.HasPrefix(href, "http") {
		fullURL = tasksURL[:idx] + href
	} else {
		fullURL = href
	}
	req, err := http.NewRequest(http.MethodDelete, fullURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create DELETE request: %w", err)
	}
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CalDAV DELETE returned %d", resp.StatusCode)
	}
	return nil
}

// getTaskFromCalDAV fetches a single VTODO by its href and returns its parsed fields.
func getTaskFromCalDAV(cfg ATCCalendarConfig, href string) (map[string]string, error) {
	tasksURL := buildTasksURL(cfg)
	idx := strings.Index(tasksURL, "/remote.php")
	var fullURL string
	if idx > 0 && !strings.HasPrefix(href, "http") {
		fullURL = tasksURL[:idx] + href
	} else {
		fullURL = href
	}
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	fields := map[string]string{}
	lines := normalizeICSLines(strings.Split(body.String(), "\n"))
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(strings.SplitN(parts[0], ";", 2)[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "SUMMARY", "UID", "STATUS", "PRIORITY", "DUE", "DTSTART", "DESCRIPTION", "LOCATION", "URL", "PERCENT-COMPLETE":
			fields[key] = cleanICSString(val)
		}
	}
	return fields, nil
}

// mergeTaskOnCalDAV fetches an existing task, overlays changed fields, and PUTs it back.
func mergeTaskOnCalDAV(cfg ATCCalendarConfig, href string, updates TaskOptions, newSummary string) error {
	fields, err := getTaskFromCalDAV(cfg, href)
	if err != nil {
		return fmt.Errorf("failed to fetch existing task: %w", err)
	}
	uid := fields["UID"]
	summary := fields["SUMMARY"]
	if newSummary != "" {
		summary = newSummary
	}
	if updates.Due != "" {
		fields["DUE"] = formatRFC3339ToICS(updates.Due)
	}
	if updates.Start != "" {
		fields["DTSTART"] = formatRFC3339ToICS(updates.Start)
	}
	if updates.Notes != "" {
		fields["DESCRIPTION"] = strings.ReplaceAll(updates.Notes, "\n", "\\n")
	}
	if updates.Location != "" {
		fields["LOCATION"] = updates.Location
	}
	if updates.Priority > 0 {
		fields["PRIORITY"] = fmt.Sprintf("%d", updates.Priority)
	}
	var extra string
	for _, k := range []string{"DUE", "DTSTART", "PRIORITY", "PERCENT-COMPLETE", "DESCRIPTION", "LOCATION", "URL"} {
		if v, ok := fields[k]; ok && v != "" {
			extra += k + ":" + v + "\r\n"
		}
	}
	icsBody := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Son of Anthon ATC//EN\r\n" +
		"BEGIN:VTODO\r\nUID:" + uid + "\r\nSUMMARY:" + summary + "\r\nSTATUS:" + fields["STATUS"] + "\r\n" +
		extra + "END:VTODO\r\nEND:VCALENDAR\r\n"

	tasksURL := buildTasksURL(cfg)
	idx := strings.Index(tasksURL, "/remote.php")
	var putURL string
	if idx > 0 && !strings.HasPrefix(href, "http") {
		putURL = caldav.FullURL(tasksURL, href)
	} else {
		putURL = href
	}
	req, err := http.NewRequest(http.MethodPut, putURL, strings.NewReader(icsBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp2, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated && resp2.StatusCode != http.StatusNoContent {
		return fmt.Errorf("CalDAV merge PUT returned %d", resp2.StatusCode)
	}
	return nil
}

// fetchICS grabs the external RFC 5545 iCal data. Supports optional HTTP Basic Auth.
func fetchICS(url, username, password string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	cfg := loadATCConfig()
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ics body: %w", err)
	}

	return normalizeICSLines(lines), nil
}

// normalizeICSLines handles RFC 5545 multiline unfolding
// "Lines of text SHOULD NOT be longer than 75 octets... Any line that
// is longer... MUST be continued on the next line... beginning with a SPACE or HTAB."
func normalizeICSLines(rawLines []string) []string {
	var folded []string
	for _, line := range rawLines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(folded) > 0 {
				lastIdx := len(folded) - 1
				folded[lastIdx] = folded[lastIdx] + line[1:]
			}
		} else {
			folded = append(folded, line)
		}
	}
	return folded
}

// parseICS securely translates the flat `.ics` RFC 5545 text lines into our XML `xCal` tree structs.
func parseICS(lines []string) *ICalendar {
	cal := &ICalendar{
		VCal: VCalendar{
			Properties: VCalProperties{
				Version: "2.0",
				Prodid:  "-//Son of Anthon//ATC Agent Sync//EN",
			},
		},
	}

	var currentEvent *VEvent
	inEvent := false

	for _, line := range lines {
		// RFC specifies keys split by colons or semicolons for parameters.
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		keyRaw := parts[0]
		val := parts[1]

		// Strip parameters (e.g., DTSTART;TZID=America/New_York -> DTSTART)
		keyBase := strings.SplitN(keyRaw, ";", 2)[0]
		key := strings.ToUpper(strings.TrimSpace(keyBase))

		switch key {
		case "BEGIN":
			if val == "VEVENT" {
				inEvent = true
				currentEvent = &VEvent{}
			}
		case "END":
			if val == "VEVENT" && inEvent {
				cal.VCal.Components.VEvents = append(cal.VCal.Components.VEvents, *currentEvent)
				inEvent = false
				currentEvent = nil
			}
		case "UID":
			if inEvent {
				currentEvent.Properties.Uid = val
			}
		case "SUMMARY":
			if inEvent {
				currentEvent.Properties.Summary = cleanICSString(val)
			}
		case "DESCRIPTION":
			if inEvent {
				currentEvent.Properties.Description = cleanICSString(val)
			}
		case "LOCATION":
			if inEvent {
				currentEvent.Properties.Location = cleanICSString(val)
			}
		case "DTSTART":
			// RFC5545 defines basic dates. For parsing properly, we format it as RFC3339 manually later,
			// or just supply it verbatim to be caught by time.Parse("20060102T150405Z")
			if inEvent {
				if len(val) == 8 {
					// Date only: 20260220
					currentEvent.Properties.DtstartDate = formatICSDate(val)
				} else {
					// Date-time: 20260220T150000Z
					currentEvent.Properties.Dtstart = formatICSDateTime(val)
				}
			}
		case "DTEND":
			if inEvent {
				if len(val) == 8 {
					currentEvent.Properties.DtendDate = formatICSDate(val)
				} else {
					currentEvent.Properties.Dtend = formatICSDateTime(val)
				}
			}
		}
	}

	return cal
}

// ICS frequently escapes commas and newlines: `\n`, `\,`
func cleanICSString(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	return s
}

// formatICSDate safely migrates 20260220 -> 2026-02-20
func formatICSDate(val string) string {
	if len(val) >= 8 {
		return fmt.Sprintf("%s-%s-%s", val[0:4], val[4:6], val[6:8])
	}
	return val
}

// formatICSDateTime securely translates Basic ISO8601 20260220T150000Z -> RFC3339
func formatICSDateTime(val string) string {
	if len(val) >= 15 && strings.Contains(val, "T") {
		date := formatICSDate(val[:8])
		timePart := val[9:]
		if len(timePart) >= 6 {
			formatted := fmt.Sprintf("%sT%s:%s:%s", date, timePart[0:2], timePart[2:4], timePart[4:6])
			if strings.HasSuffix(val, "Z") {
				return formatted + "Z"
			}
			// Assumption: local timezone string format parsing handles off-sets.
			return formatted + "Z"
		}
	}
	return val
}
