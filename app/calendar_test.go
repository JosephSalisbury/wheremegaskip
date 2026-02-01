package app

import (
	"strings"
	"testing"
	"time"
)

func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		expected float64
		epsilon  float64
	}{
		{
			name:     "Same point",
			lat1:     51.4567,
			lon1:     -0.1910,
			lat2:     51.4567,
			lon2:     -0.1910,
			expected: 0,
			epsilon:  0.001,
		},
		{
			name:     "London to Paris approximately",
			lat1:     51.5074,
			lon1:     -0.1278,
			lat2:     48.8566,
			lon2:     2.3522,
			expected: 343, // approximately 343 km
			epsilon:  5,
		},
		{
			name:     "Short distance in Wandsworth",
			lat1:     51.4567,
			lon1:     -0.1910,
			lat2:     51.4600,
			lon2:     -0.1850,
			expected: 0.55, // approximately 0.5 km
			epsilon:  0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := haversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.epsilon {
				t.Errorf("haversineDistance() = %v, expected %v (±%v)", result, tt.expected, tt.epsilon)
			}
		})
	}
}

func TestGroupSkipsByDate(t *testing.T) {
	date1 := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2025, 3, 22, 0, 0, 0, 0, time.UTC)

	skips := []SkipLocation{
		{Address: "Location A", Postcode: "SW11 1AA", Date: date1},
		{Address: "Location B", Postcode: "SW11 1BB", Date: date1},
		{Address: "Location C", Postcode: "SW11 1CC", Date: date2},
	}

	groups := groupSkipsByDate(skips)

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(groups))
	}

	if len(groups[date1]) != 2 {
		t.Errorf("Expected 2 skips for date1, got %d", len(groups[date1]))
	}

	if len(groups[date2]) != 1 {
		t.Errorf("Expected 1 skip for date2, got %d", len(groups[date2]))
	}
}

func TestFindNearestSkipForDate(t *testing.T) {
	date := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	skips := []SkipLocation{
		{Address: "Far Location", Postcode: "SW11 1AA", Date: date, Latitude: 51.5, Longitude: -0.1},
		{Address: "Near Location", Postcode: "SW11 1BB", Date: date, Latitude: 51.457, Longitude: -0.191},
		{Address: "Different Date", Postcode: "SW11 1CC", Date: date.AddDate(0, 0, 7), Latitude: 51.456, Longitude: -0.190},
	}

	userLat := 51.4567
	userLng := -0.1910

	nearest := findNearestSkipForDate(skips, date, userLat, userLng)

	if nearest == nil {
		t.Fatal("Expected to find a nearest skip")
	}

	if nearest.Address != "Near Location" {
		t.Errorf("Expected nearest to be 'Near Location', got '%s'", nearest.Address)
	}
}

func TestFindNearestSkipForDateNoMatches(t *testing.T) {
	date := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	searchDate := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)

	skips := []SkipLocation{
		{Address: "Location A", Postcode: "SW11 1AA", Date: date, Latitude: 51.457, Longitude: -0.191},
	}

	nearest := findNearestSkipForDate(skips, searchDate, 51.4567, -0.1910)

	if nearest != nil {
		t.Error("Expected nil when no skips match the date")
	}
}

func TestEscapeICalText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple text", "Simple text"},
		{"Text, with comma", "Text\\, with comma"},
		{"Text; with semicolon", "Text\\; with semicolon"},
		{"Text\nwith newline", "Text\\nwith newline"},
		{"Text\\with backslash", "Text\\\\with backslash"},
		{"Multiple, special; chars\n", "Multiple\\, special\\; chars\\n"},
	}

	for _, tt := range tests {
		result := escapeICalText(tt.input)
		if result != tt.expected {
			t.Errorf("escapeICalText(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestGenerateUID(t *testing.T) {
	date := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	uid1 := generateUID(date)
	uid2 := generateUID(date)

	// Same date should produce same UID
	if uid1 != uid2 {
		t.Errorf("Same date should produce same UID, got %s and %s", uid1, uid2)
	}

	// Should end with @wheremegaskip.com
	if !strings.HasSuffix(uid1, "@wheremegaskip.com") {
		t.Errorf("UID should end with @wheremegaskip.com, got %s", uid1)
	}

	// Different date should produce different UID
	differentDate := time.Date(2025, 3, 16, 0, 0, 0, 0, time.UTC)
	uid3 := generateUID(differentDate)
	if uid1 == uid3 {
		t.Error("Different dates should produce different UIDs")
	}
}

func TestGenerateICalFeed(t *testing.T) {
	events := []CalendarEvent{
		{
			Date:        time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
			Title:       "Wandsworth Mega Skip",
			Description: "https://wheremegaskip.com",
			Location:    "",
		},
		{
			Date:        time.Date(2025, 3, 22, 0, 0, 0, 0, time.UTC),
			Title:       "Wandsworth Mega Skip",
			Description: "https://wheremegaskip.com",
			Location:    "Pountney Road, SW11 5TU, London, UK",
		},
	}

	ical := generateICalFeed(events)

	// Check required iCal components
	requiredStrings := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//WhereMegaSkip//Calendar//EN",
		"BEGIN:VTIMEZONE",
		"TZID:Europe/London",
		"END:VTIMEZONE",
		"BEGIN:VEVENT",
		"END:VEVENT",
		"END:VCALENDAR",
		"DTSTART;TZID=Europe/London:20250315T090000",
		"DTEND;TZID=Europe/London:20250315T120000",
		"SUMMARY:Wandsworth Mega Skip",
		"LOCATION:Pountney Road\\, SW11 5TU\\, London\\, UK",
	}

	for _, s := range requiredStrings {
		if !strings.Contains(ical, s) {
			t.Errorf("iCal feed should contain %q", s)
		}
	}

	// Verify line endings are CRLF
	if !strings.Contains(ical, "\r\n") {
		t.Error("iCal feed should use CRLF line endings")
	}
}

func TestGenerateICalFeedNoLocation(t *testing.T) {
	events := []CalendarEvent{
		{
			Date:        time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
			Title:       "Wandsworth Mega Skip",
			Description: "Test description",
			Location:    "",
		},
	}

	ical := generateICalFeed(events)

	// Events without location should not have LOCATION field
	if strings.Contains(ical, "LOCATION:") {
		t.Error("iCal feed should not contain LOCATION field for events without location")
	}
}
