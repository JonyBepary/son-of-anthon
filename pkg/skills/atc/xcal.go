package atc

import "encoding/xml"

// ICalendar represents the root of an RFC 6321 xCal document
type ICalendar struct {
	XMLName xml.Name  `xml:"icalendar"`
	VCal    VCalendar `xml:"vcalendar"`
}

type VCalendar struct {
	Properties VCalProperties `xml:"properties"`
	Components Components     `xml:"components"`
}

type VCalProperties struct {
	Version string `xml:"version>text"`
	Prodid  string `xml:"prodid>text"`
}

type Components struct {
	VEvents []VEvent `xml:"vevent"`
	VTodos  []VTodo  `xml:"vtodo"`
}

// VEvent represents a calendar event
type VEvent struct {
	Properties VEventProperties `xml:"properties"`
}

type VEventProperties struct {
	Uid         string `xml:"uid>text"`
	Dtstamp     string `xml:"dtstamp>date-time"`
	Dtstart     string `xml:"dtstart>date-time"`
	DtstartDate string `xml:"dtstart>date"` // For all-day events
	Dtend       string `xml:"dtend>date-time"`
	DtendDate   string `xml:"dtend>date"` // For all-day events
	Summary     string `xml:"summary>text"`
	Description string `xml:"description>text"`
	Location    string `xml:"location>text"`
}

// VTodo represents a task
type VTodo struct {
	Properties VTodoProperties `xml:"properties"`
}

type VTodoProperties struct {
	Uid         string `xml:"uid>text"`
	Dtstamp     string `xml:"dtstamp>date-time"`
	Summary     string `xml:"summary>text"`
	Description string `xml:"description>text"`
	Status      string `xml:"status>text"`      // e.g., NEEDS-ACTION, COMPLETED, IN-PROCESS, CANCELLED
	Priority    int    `xml:"priority>integer"` // 0 (undefined), 1 (highest) to 9 (lowest)
	Due         string `xml:"due>date-time"`    // Deadline
	DueDate     string `xml:"due>date"`         // Deadline (date only)
	Categories  string `xml:"categories>text"`  // e.g., Today, Tomorrow, Someday
}
