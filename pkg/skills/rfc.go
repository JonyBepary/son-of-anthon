package skills

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NormalizeURL strips tracking parameters before hashing so the same
// logical resource always produces the same uuid12.
func NormalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	trackingKeys := []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "ref", "context", "source"}
	for _, k := range trackingKeys {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	// Drop fragment
	u.Fragment = ""
	return u.String()
}

// UUID12 returns the first 12 hex characters of SHA-256(normalizedURL).
// 48-bit space; Birthday collision at ~67M entries.
func UUID12(rawURL string) string {
	normalized := NormalizeURL(rawURL)
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum)[:12]
}

// sanitizeField strips pipe characters and newlines from a field value
// to prevent delimiter/newline injection.
func sanitizeField(s string) string {
	s = strings.ReplaceAll(s, "|", "-")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// EncodeRecord formats one RFC cache line:
//
//	[type:uuid12:tag] title | YYYYMMDD | url
func EncodeRecord(recType, rawURL, title, tag, date string) string {
	id := UUID12(rawURL)

	// Sanitize variable fields
	title = sanitizeField(title)
	tag = sanitizeField(tag)
	// URL: never truncate, only strip newlines
	rawURL = strings.ReplaceAll(rawURL, "\r", "")
	rawURL = strings.ReplaceAll(rawURL, "\n", "")

	// Truncate title to 80 chars; tag to 20 chars
	if len(title) > 80 {
		title = title[:77] + "..."
	}
	if len(tag) > 20 {
		tag = tag[:20]
	}

	// Normalise date to YYYYMMDD
	dateCompact := strings.ReplaceAll(strings.ReplaceAll(date, "-", ""), "/", "")
	if len(dateCompact) > 8 {
		dateCompact = dateCompact[:8]
	}
	if dateCompact == "" {
		dateCompact = time.Now().Format("20060102")
	}

	return fmt.Sprintf("[%s:%s:%s] %s | %s | %s", recType, id, tag, title, dateCompact, rawURL)
}

// ParseTTL parses a TTL string like "6h", "24h", "72h" into a duration.
func ParseTTL(ttl string) time.Duration {
	ttl = strings.ToLower(strings.TrimSpace(ttl))
	hours, err := strconv.Atoi(strings.TrimSuffix(ttl, "h"))
	if err != nil || hours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(hours) * time.Hour
}

// ParseRFCFile reads, TTL-checks, passively GCs, and returns up to
// maxRecords record lines from an RFC cache file.
// Returns nil,nil when the file doesn't exist (not an error).
func ParseRFCFile(path string, maxRecords int) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var tsVal, ttlVal string
	var records []string

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TS:") {
			tsVal = strings.TrimSpace(strings.TrimPrefix(line, "TS:"))
		} else if strings.HasPrefix(line, "TTL:") {
			ttlVal = strings.TrimSpace(strings.TrimPrefix(line, "TTL:"))
		} else if strings.HasPrefix(line, "[") {
			records = append(records, line)
		}
	}

	// TTL check + passive GC
	if tsVal != "" && ttlVal != "" {
		ts, err := time.Parse(time.RFC3339, tsVal)
		if err == nil {
			ttl := ParseTTL(ttlVal)
			if time.Since(ts) > ttl {
				os.Remove(path) // passive GC
				return nil, nil
			}
		}
	}

	// Cap to maxRecords
	if maxRecords > 0 && len(records) > maxRecords {
		records = records[:maxRecords]
	}
	return records, nil
}

// WriteRFCFile merges newLines into the existing RFC file by uuid12,
// then atomically overwrites using a .tmp file + os.Rename.
func WriteRFCFile(path, agent, ttl string, newLines []string) error {
	// Load existing records, merge by uuid12 (new wins)
	existing := make(map[string]string) // uuid12 → full line
	var order []string

	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				id := extractUUID12(line)
				if id != "" && existing[id] == "" {
					order = append(order, id)
				}
				existing[id] = line
			}
		}
	}

	for _, line := range newLines {
		id := extractUUID12(line)
		if id == "" {
			continue
		}
		if existing[id] == "" {
			order = append(order, id)
		}
		existing[id] = line // new wins
	}

	// Build file content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("AGENT:  %s\n", agent))
	sb.WriteString(fmt.Sprintf("TS:     %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("TTL:    %s\n", ttl))
	sb.WriteString(fmt.Sprintf("COUNT:  %d\n", len(order)))
	sb.WriteString("\n")
	for _, id := range order {
		sb.WriteString(existing[id] + "\n")
	}

	// Atomic write
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// extractUUID12 pulls the uuid12 from a record line like "[type:uuid12:tag] ..."
func extractUUID12(line string) string {
	end := strings.Index(line, "]")
	if end < 0 || !strings.HasPrefix(line, "[") {
		return ""
	}
	parts := strings.SplitN(line[1:end], ":", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
