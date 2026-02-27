// Package caldav provides shared utilities for Nextcloud CalDAV and WebDAV operations.
package caldav

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// NextcloudConfig is the minimal interface required for URL construction.
type NextcloudConfig interface {
	GetHost() string
	GetUsername() string
}

// BuildTasksURL constructs the CalDAV Tasks collection URL.
// e.g. https://host/remote.php/dav/calendars/user/tasks/
func BuildTasksURL(host, username string) string {
	base := strings.TrimRight(host, "/")
	return fmt.Sprintf("%s/remote.php/dav/calendars/%s/tasks/", base, url.PathEscape(username))
}

// BuildCalendarURL constructs the CalDAV personal calendar URL.
// e.g. https://host/remote.php/dav/calendars/user/personal/
func BuildCalendarURL(host, username string) string {
	base := strings.TrimRight(host, "/")
	return fmt.Sprintf("%s/remote.php/dav/calendars/%s/personal/", base, url.PathEscape(username))
}

// BuildFilesURL constructs the WebDAV files base URL.
// e.g. https://host/remote.php/webdav/
func BuildFilesURL(host string) string {
	base := strings.TrimRight(host, "/")
	return fmt.Sprintf("%s/remote.php/webdav/", base)
}

// BuildDeckURL constructs the Nextcloud Deck API base URL.
// e.g. https://host/index.php/apps/deck/api/v1.0/
func BuildDeckURL(host string) string {
	base := strings.TrimRight(host, "/")
	return fmt.Sprintf("%s/index.php/apps/deck/api/v1.0/", base)
}

// FormatRFC3339ToICS converts an RFC3339 timestamp to ICS UTC format.
// RFC 5545 requires absolute timestamps to use UTC with a 'Z' suffix.
// e.g. "2026-02-24T20:00:00+06:00" → "20260224T140000Z"
func FormatRFC3339ToICS(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format("20060102T150405Z")
	}
	// Fallback: strip dashes and colons (handles already-partial ICS strings)
	replacer := strings.NewReplacer("-", "", ":", "")
	return replacer.Replace(ts)
}

// FullURL reconstructs a full absolute URL from a base tasks URL and a relative href.
// Nextcloud PROPFIND returns hrefs as relative paths.
func FullURL(tasksURL, href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	idx := strings.Index(tasksURL, "/remote.php")
	if idx > 0 {
		return tasksURL[:idx] + href
	}
	return href
}
