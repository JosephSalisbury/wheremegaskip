package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

//go:embed index.html
var htmlTemplate string

// indexTemplate is parsed once at package init for better performance
var indexTemplate = template.Must(template.New("index").Parse(htmlTemplate))

// SkipLocation represents a megaskip location with its details
type SkipLocation struct {
	Address   string    `json:"address"`
	Postcode  string    `json:"postcode"`
	Date      time.Time `json:"date"`
	DateStr   string    `json:"dateStr"` // Human-readable date
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lng"`
}

// Cache holds the skip locations with expiry
type Cache struct {
	data      []SkipLocation
	timestamp time.Time
	mu        sync.RWMutex
	ttl       time.Duration
}

var cache = &Cache{
	ttl: 1 * time.Hour, // Default 1 hour, configurable via env var
}

// InitCache sets up the cache with the configured TTL
func InitCache() {
	if ttl := os.Getenv("CACHE_TTL_MINUTES"); ttl != "" {
		if minutes, err := time.ParseDuration(ttl + "m"); err == nil {
			cache.ttl = minutes
			log.Printf("Cache TTL set to %v", cache.ttl)
		}
	}
}

// HandleIndex handles the main page request
func HandleIndex(w http.ResponseWriter, r *http.Request) {
	// Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' 'unsafe-inline' https://unpkg.com; "+
			"style-src 'self' 'unsafe-inline' https://unpkg.com; "+
			"img-src 'self' data: https://*.openstreetmap.org https://*.tile.openstreetmap.org; "+
			"connect-src 'self' https://nominatim.openstreetmap.org; "+
			"font-src 'self' data:;")

	// Get skip locations (from cache or fetch fresh)
	locations, err := getSkipLocations()
	if err != nil {
		log.Printf("Error getting skip locations: %v", err)
		http.Error(w, "Failed to fetch skip locations", http.StatusInternalServerError)
		return
	}

	// Render the page with locations embedded
	renderPage(w, locations)
}

func getSkipLocations() ([]SkipLocation, error) {
	cache.mu.RLock()
	if cache.data != nil && time.Since(cache.timestamp) < cache.ttl {
		defer cache.mu.RUnlock()
		log.Println("Serving from cache")
		return cache.data, nil
	}
	cache.mu.RUnlock()

	// Need to fetch fresh data
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check after acquiring write lock
	if cache.data != nil && time.Since(cache.timestamp) < cache.ttl {
		return cache.data, nil
	}

	log.Println("Fetching fresh data from council website")
	locations, err := scrapeCouncilWebsite()
	if err != nil {
		return nil, fmt.Errorf("scraping failed: %w", err)
	}

	cache.data = locations
	cache.timestamp = time.Now()

	return locations, nil
}

func scrapeCouncilWebsite() ([]SkipLocation, error) {
	url := "https://www.wandsworth.gov.uk/mega-skip-days"

	// Fetch the page
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var locations []SkipLocation
	now := time.Now()

	// Find all h3 elements that contain dates (e.g., "Saturday 31 January")
	doc.Find("h3").Each(func(i int, s *goquery.Selection) {
		dateText := s.Text()

		// Try to parse the date
		date, err := parseSkipDate(dateText, now.Year())
		if err != nil {
			// Not a date heading, skip
			return
		}

		// Find the next sibling or nearby elements containing the location list
		// Look for the next paragraph or list
		nextEl := s.Next()
		for nextEl.Length() > 0 {
			// Check if this is a list or contains location info
			text := nextEl.Text()
			if text == "" || nextEl.Is("h2") || nextEl.Is("h3") {
				break
			}

			// Parse locations from this element
			locs := parseLocations(nextEl, date, dateText)
			locations = append(locations, locs...)

			nextEl = nextEl.Next()
		}
	})

	// Filter to only upcoming dates
	filtered := []SkipLocation{}
	for _, loc := range locations {
		if loc.Date.After(now) || loc.Date.Equal(now.Truncate(24*time.Hour)) {
			filtered = append(filtered, loc)
		}
	}

	// Geocode each location
	log.Printf("Geocoding %d locations...", len(filtered))
	for i := range filtered {
		lat, lng, err := geocodePostcode(filtered[i].Postcode)
		if err != nil {
			log.Printf("Failed to geocode %s: %v", filtered[i].Postcode, err)
			continue
		}
		filtered[i].Latitude = lat
		filtered[i].Longitude = lng
		log.Printf("Geocoded %s: %.4f, %.4f", filtered[i].Postcode, lat, lng)

		// Respect Nominatim rate limit (1 request per second recommended)
		if i < len(filtered)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	log.Println("Geocoding complete")

	return filtered, nil
}

func parseSkipDate(dateStr string, year int) (time.Time, error) {
	// Try to parse dates like "Saturday 31 January"
	// We'll try multiple formats
	formats := []string{
		"Monday 2 January",
		"Monday 02 January",
	}

	dateStr = fmt.Sprintf("%s %d", dateStr, year)

	for _, format := range formats {
		formatWithYear := format + " 2006"
		t, err := time.Parse(formatWithYear, dateStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("not a valid date format")
}

func parseLocations(el *goquery.Selection, date time.Time, dateStr string) []SkipLocation {
	var locations []SkipLocation

	// Look for bullet points or list items
	el.Find("li").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		loc := parseLocationLine(text, date, dateStr)
		if loc.Address != "" {
			locations = append(locations, loc)
		}
	})

	// If no list items found, try parsing text lines
	if len(locations) == 0 {
		text := el.Text()
		// Try parsing the whole text as one location
		if loc := parseLocationLine(text, date, dateStr); loc.Address != "" {
			locations = append(locations, loc)
		}
	}

	return locations
}

func parseLocationLine(line string, date time.Time, dateStr string) SkipLocation {
	// Format is typically: "Location Name, POSTCODE"
	// Example: "Pountney Road, SW11 5TU"

	// Remove bullet points and trim
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "•")
	line = strings.TrimPrefix(line, "-")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimSpace(line)

	if line == "" {
		return SkipLocation{}
	}

	// Split by comma to separate address from postcode
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return SkipLocation{}
	}

	address := strings.TrimSpace(parts[0])
	postcode := strings.TrimSpace(strings.Join(parts[1:], ","))

	// UK postcode pattern validation (basic)
	postcodePattern := regexp.MustCompile(`^[A-Z]{1,2}\d{1,2}[A-Z]?\s?\d[A-Z]{2}$`)
	if !postcodePattern.MatchString(strings.ToUpper(postcode)) {
		// Try to extract postcode from the end of the string
		words := strings.Fields(postcode)
		if len(words) >= 2 {
			// Last two words might be postcode
			potentialPostcode := strings.Join(words[len(words)-2:], " ")
			if postcodePattern.MatchString(strings.ToUpper(potentialPostcode)) {
				postcode = potentialPostcode
			}
		}
	}

	return SkipLocation{
		Address:  address,
		Postcode: strings.ToUpper(postcode),
		Date:     date,
		DateStr:  dateStr,
	}
}

// geocodePostcode calls Nominatim API to get lat/lng for a postcode
func geocodePostcode(postcode string) (float64, float64, error) {
	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s+London+UK&format=json&limit=1&countrycodes=gb",
		url.QueryEscape(postcode))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "WhereMegaSkip/1.0 (https://github.com/JosephSalisbury/wheremegaskip)")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch geocode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, 0, fmt.Errorf("geocode API returned status %d", resp.StatusCode)
	}

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, fmt.Errorf("failed to decode geocode response: %w", err)
	}

	if len(results) == 0 {
		return 0, 0, fmt.Errorf("no geocode results for postcode %s", postcode)
	}

	var lat, lng float64
	if _, err := fmt.Sscanf(results[0].Lat, "%f", &lat); err != nil {
		return 0, 0, fmt.Errorf("failed to parse latitude: %w", err)
	}
	if _, err := fmt.Sscanf(results[0].Lon, "%f", &lng); err != nil {
		return 0, 0, fmt.Errorf("failed to parse longitude: %w", err)
	}

	return lat, lng, nil
}

func renderPage(w http.ResponseWriter, locations []SkipLocation) {
	// Convert locations to JSON for embedding
	locationsJSON, err := json.Marshal(locations)
	if err != nil {
		http.Error(w, "Failed to encode locations", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Locations":     template.JS(locationsJSON),
		"LocationCount": len(locations),
	}

	if err := indexTemplate.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}
