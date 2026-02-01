package app

import (
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CalendarEvent represents a single calendar event
type CalendarEvent struct {
	Date        time.Time
	Title       string
	Description string
	Location    string
}

// haversineDistance calculates the distance in kilometers between two points
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// groupSkipsByDate groups skip locations by their date
func groupSkipsByDate(skips []SkipLocation) map[time.Time][]SkipLocation {
	groups := make(map[time.Time][]SkipLocation)
	for _, skip := range skips {
		// Normalize to start of day
		date := time.Date(skip.Date.Year(), skip.Date.Month(), skip.Date.Day(), 0, 0, 0, 0, time.UTC)
		groups[date] = append(groups[date], skip)
	}
	return groups
}

// findNearestSkipForDate finds the closest skip location for a given date
func findNearestSkipForDate(skips []SkipLocation, date time.Time, userLat, userLng float64) *SkipLocation {
	var nearest *SkipLocation
	minDist := math.MaxFloat64

	for i, skip := range skips {
		skipDate := time.Date(skip.Date.Year(), skip.Date.Month(), skip.Date.Day(), 0, 0, 0, 0, time.UTC)
		targetDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

		if !skipDate.Equal(targetDate) {
			continue
		}

		dist := haversineDistance(userLat, userLng, skip.Latitude, skip.Longitude)
		if dist < minDist {
			minDist = dist
			nearest = &skips[i]
		}
	}

	return nearest
}

// escapeICalText escapes special characters for iCal format
func escapeICalText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, ";", "\\;")
	text = strings.ReplaceAll(text, ",", "\\,")
	text = strings.ReplaceAll(text, "\n", "\\n")
	return text
}

// generateUID creates a unique ID for an event based on the date
func generateUID(date time.Time) string {
	dateStr := date.Format("2006-01-02")
	hash := sha256.Sum256([]byte(dateStr))
	return fmt.Sprintf("%x@wheremegaskip.com", hash[:8])
}

// generateICalFeed generates an RFC 5545 compliant iCal feed
func generateICalFeed(events []CalendarEvent) string {
	var sb strings.Builder

	// Calendar header
	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//WhereMegaSkip//Calendar//EN\r\n")
	sb.WriteString("CALSCALE:GREGORIAN\r\n")
	sb.WriteString("METHOD:PUBLISH\r\n")
	sb.WriteString("X-WR-CALNAME:Wandsworth Megaskip\r\n")
	sb.WriteString("X-WR-TIMEZONE:Europe/London\r\n")

	// VTIMEZONE component for Europe/London
	sb.WriteString("BEGIN:VTIMEZONE\r\n")
	sb.WriteString("TZID:Europe/London\r\n")
	sb.WriteString("BEGIN:DAYLIGHT\r\n")
	sb.WriteString("TZOFFSETFROM:+0000\r\n")
	sb.WriteString("TZOFFSETTO:+0100\r\n")
	sb.WriteString("TZNAME:BST\r\n")
	sb.WriteString("DTSTART:19700329T010000\r\n")
	sb.WriteString("RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU\r\n")
	sb.WriteString("END:DAYLIGHT\r\n")
	sb.WriteString("BEGIN:STANDARD\r\n")
	sb.WriteString("TZOFFSETFROM:+0100\r\n")
	sb.WriteString("TZOFFSETTO:+0000\r\n")
	sb.WriteString("TZNAME:GMT\r\n")
	sb.WriteString("DTSTART:19701025T020000\r\n")
	sb.WriteString("RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU\r\n")
	sb.WriteString("END:STANDARD\r\n")
	sb.WriteString("END:VTIMEZONE\r\n")

	// Generate events
	now := time.Now().UTC()
	dtstamp := now.Format("20060102T150405Z")

	for _, event := range events {
		sb.WriteString("BEGIN:VEVENT\r\n")
		sb.WriteString(fmt.Sprintf("UID:%s\r\n", generateUID(event.Date)))
		sb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", dtstamp))

		// Event start: 9am London time
		dtstart := fmt.Sprintf("%04d%02d%02dT090000",
			event.Date.Year(), event.Date.Month(), event.Date.Day())
		sb.WriteString(fmt.Sprintf("DTSTART;TZID=Europe/London:%s\r\n", dtstart))

		// Event end: 12pm London time
		dtend := fmt.Sprintf("%04d%02d%02dT120000",
			event.Date.Year(), event.Date.Month(), event.Date.Day())
		sb.WriteString(fmt.Sprintf("DTEND;TZID=Europe/London:%s\r\n", dtend))

		sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICalText(event.Title)))
		sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICalText(event.Description)))

		if event.Location != "" {
			sb.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICalText(event.Location)))
		}

		sb.WriteString("END:VEVENT\r\n")
	}

	sb.WriteString("END:VCALENDAR\r\n")
	return sb.String()
}

// HandleCalendarDefault handles requests to /calendar.ics (default feed, no location)
func HandleCalendarDefault(w http.ResponseWriter, r *http.Request) {
	locations, err := getSkipLocations()
	if err != nil {
		http.Error(w, "Failed to generate calendar", http.StatusInternalServerError)
		return
	}

	// Group by date and create one event per date
	groups := groupSkipsByDate(locations)

	var events []CalendarEvent
	for date := range groups {
		events = append(events, CalendarEvent{
			Date:        date,
			Title:       "Wandsworth Megaskip",
			Description: "Opens 9am, closes at 12 noon or when full.\\nhttps://wheremegaskip.com",
			Location:    "",
		})
	}

	// Sort events by date
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.Before(events[j].Date)
	})

	ical := generateICalFeed(events)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"wandsworth-megaskip.ics\"")
	w.Write([]byte(ical))
}

// HandleCalendarPostcode handles requests to /calendar/{postcode}.ics (personalized feed)
func HandleCalendarPostcode(w http.ResponseWriter, r *http.Request) {
	// Extract postcode from path
	path := r.URL.Path
	if !strings.HasPrefix(path, "/calendar/") || !strings.HasSuffix(path, ".ics") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Get the postcode portion
	postcodeEncoded := strings.TrimPrefix(path, "/calendar/")
	postcodeEncoded = strings.TrimSuffix(postcodeEncoded, ".ics")

	postcode, err := url.QueryUnescape(postcodeEncoded)
	if err != nil {
		http.Error(w, "Invalid postcode encoding", http.StatusBadRequest)
		return
	}

	// Validate postcode format (basic UK postcode pattern)
	postcodePattern := regexp.MustCompile(`^[A-Za-z]{1,2}\d{1,2}[A-Za-z]?\s?\d[A-Za-z]{2}$`)
	if !postcodePattern.MatchString(postcode) {
		http.Error(w, "Invalid postcode format", http.StatusBadRequest)
		return
	}

	// Geocode the user's postcode
	userLat, userLng, err := geocodePostcode(postcode)
	if err != nil {
		http.Error(w, "Could not find postcode location", http.StatusBadRequest)
		return
	}

	locations, err := getSkipLocations()
	if err != nil {
		http.Error(w, "Failed to generate calendar", http.StatusInternalServerError)
		return
	}

	// Group by date and find nearest skip for each date
	groups := groupSkipsByDate(locations)

	var events []CalendarEvent
	for date, skips := range groups {
		nearest := findNearestSkipForDate(skips, date, userLat, userLng)

		var location string
		if nearest != nil {
			location = fmt.Sprintf("%s, %s, London, UK", nearest.Address, nearest.Postcode)
		}

		events = append(events, CalendarEvent{
			Date:        date,
			Title:       "Wandsworth Megaskip",
			Description: "Opens 9am, closes at 12 noon or when full.\\nhttps://wheremegaskip.com",
			Location:    location,
		})
	}

	// Sort events by date
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date.Before(events[j].Date)
	})

	ical := generateICalFeed(events)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"wandsworth-megaskip.ics\"")
	w.Write([]byte(ical))
}
