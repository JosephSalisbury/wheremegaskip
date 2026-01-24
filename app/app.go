package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

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

func renderPage(w http.ResponseWriter, locations []SkipLocation) {
	// Convert locations to JSON for embedding
	locationsJSON, err := json.Marshal(locations)
	if err != nil {
		http.Error(w, "Failed to encode locations", http.StatusInternalServerError)
		return
	}

	tmpl := template.Must(template.New("index").Parse(htmlTemplate))
	data := map[string]interface{}{
		"Locations":     template.JS(locationsJSON),
		"LocationCount": len(locations),
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=5.0, user-scalable=yes">
    <meta name="theme-color" content="#0074A2">
    <meta name="description" content="Find your nearest Wandsworth Mega Skip location with live map and directions">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="default">
    <title>Where Mega Skip?</title>
    <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" />
    <style>
        /* Wandsworth-inspired colors: teal/blue primary, coral accents */
        * {
            box-sizing: border-box;
        }
        
        body {
            margin: 0;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            color: #333;
            background: #f5f5f5;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }
        
        #container {
            max-width: 900px;
            margin: 0 auto;
            padding: 20px;
        }
        
        @media (max-width: 768px) {
            #container {
                padding: 12px;
            }
        }
        
        #header {
            background: linear-gradient(135deg, #0074A2 0%, #00A1C9 100%);
            color: white;
            padding: 30px;
            border-radius: 8px;
            text-align: center;
            margin-bottom: 20px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        
        @media (max-width: 768px) {
            #header {
                padding: 20px 15px;
                margin-bottom: 12px;
                border-radius: 6px;
            }
        }
        
        h1 {
            margin: 0 0 10px 0;
            font-size: 32px;
            font-weight: 600;
        }
        
        @media (max-width: 768px) {
            h1 {
                font-size: 24px;
                margin: 0 0 8px 0;
            }
        }
        
        #subtitle {
            font-size: 16px;
            opacity: 0.95;
        }
        
        @media (max-width: 768px) {
            #subtitle {
                font-size: 14px;
            }
        }
        
        #date-banner {
            background: white;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        @media (max-width: 768px) {
            #date-banner {
                padding: 15px;
                margin-bottom: 12px;
                border-radius: 6px;
            }
        }
        
        #date-banner h2 {
            margin: 0 0 10px 0;
            color: #0074A2;
            font-size: 20px;
        }
        
        @media (max-width: 768px) {
            #date-banner h2 {
                font-size: 16px;
            }
        }
        
        #date-info {
            margin-bottom: 15px;
            padding-bottom: 15px;
            border-bottom: 1px solid #e0e0e0;
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 20px;
        }
        
        @media (max-width: 768px) {
            #date-info {
                flex-direction: column;
                align-items: flex-start;
                gap: 10px;
                margin-bottom: 12px;
                padding-bottom: 12px;
            }
        }
        
        #date-info h2 {
            white-space: nowrap;
            margin: 0;
            flex-shrink: 0;
        }
        
        @media (max-width: 768px) {
            #date-info h2 {
                white-space: normal;
            }
        }
        
        .time-info {
            color: #666;
            font-size: 13px;
            font-weight: normal;
            text-align: right;
            flex-shrink: 1;
            max-width: 280px;
        }
        
        @media (max-width: 768px) {
            .time-info {
                text-align: left;
                max-width: 100%;
                font-size: 12px;
            }
        }
        
        #date-banner.disabled {
            opacity: 0.5;
            pointer-events: none;
        }
        
        .control-group {
            margin-bottom: 15px;
            display: flex;
            gap: 10px;
            align-items: center;
        }
        
        @media (max-width: 768px) {
            .control-group {
                flex-direction: column;
                align-items: stretch;
                gap: 8px;
                margin-bottom: 0;
            }
            
            .control-group > span {
                display: none; /* Hide 'or' separator on mobile */
            }
        }
        
        .control-group:last-child {
            margin-bottom: 0;
        }
        
        .control-group.stacked {
            flex-direction: column;
            align-items: stretch;
        }
        
        label {
            display: block;
            font-weight: 500;
            margin-bottom: 5px;
            font-size: 14px;
        }
        
        input[type="text"] {
            width: 100%;
            padding: 10px;
            border: 2px solid #e0e0e0;
            border-radius: 4px;
            font-size: 14px;
            -webkit-appearance: none;
            appearance: none;
        }
        
        @media (max-width: 768px) {
            input[type="text"] {
                padding: 14px 12px;
                font-size: 16px; /* Prevents zoom on iOS */
                min-height: 48px;
            }
        }
        
        input[type="text"]:focus {
            outline: none;
            border-color: #0074A2;
        }
        
        button {
            background: #0074A2;
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 4px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: background 0.2s;
            white-space: nowrap;
            -webkit-tap-highlight-color: rgba(0, 0, 0, 0.1);
        }
        
        @media (max-width: 768px) {
            button {
                width: 100%;
                padding: 14px 20px;
                font-size: 15px;
                min-height: 48px; /* Touch-friendly target size */
            }
        }
        
        button:hover {
            background: #005580;
        }
        
        @media (hover: none) {
            button:hover {
                background: #0074A2; /* Disable hover on touch devices */
            }
            
            button:active {
                background: #005580;
            }
        }
        
        button:disabled {
            background: #ccc;
            cursor: not-allowed;
        }
        
        #map-container {
            background: white;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
            position: relative;
        }
        
        #map-loading {
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(255, 255, 255, 0.9);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 1000;
            backdrop-filter: blur(2px);
        }
        
        #map-loading.hidden {
            display: none;
        }
        
        .loading-spinner {
            text-align: center;
        }
        
        .loading-spinner h3 {
            margin: 10px 0;
            color: #0074A2;
            font-size: 18px;
        }
        
        .spinner {
            border: 4px solid #f3f3f3;
            border-top: 4px solid #0074A2;
            border-radius: 50%;
            width: 50px;
            height: 50px;
            animation: spin 1s linear infinite;
            margin: 0 auto;
        }
        
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        
        #map {
            height: 500px;
            width: 100%;
        }
        
        #nearest-info {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.15);
            margin-bottom: 20px;
            border-left: 4px solid #FF7043;
            display: none;
            cursor: pointer;
            transition: all 0.2s ease;
            -webkit-tap-highlight-color: rgba(0, 0, 0, 0.05);
        }
        
        @media (max-width: 768px) {
            #nearest-info {
                padding: 15px;
                margin-bottom: 12px;
                border-radius: 6px;
            }
        }
        
        #nearest-info:hover {
            box-shadow: 0 4px 12px rgba(0,0,0,0.2);
            transform: translateY(-2px);
        }
        
        @media (hover: none) {
            #nearest-info:hover {
                transform: none;
            }
            
            #nearest-info:active {
                transform: scale(0.98);
            }
        }
        
        #nearest-info h3 {
            margin-top: 0;
            color: #FF7043;
            font-size: 22px;
        }
        
        @media (max-width: 768px) {
            #nearest-info h3 {
                font-size: 18px;
            }
        }
        
        #nearest-info.visible {
            display: block;
        }
        
        .nearest-detail {
            margin: 10px 0;
            font-size: 16px;
        }
        
        @media (max-width: 768px) {
            .nearest-detail {
                font-size: 14px;
                margin: 8px 0;
            }
        }
        
        .nearest-detail strong {
            font-weight: 600;
        }
        
        #skip-list {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        @media (max-width: 768px) {
            #skip-list {
                padding: 15px;
                border-radius: 6px;
            }
        }
        
        #skip-list h3 {
            margin-top: 0;
            color: #0074A2;
            font-size: 20px;
        }
        
        @media (max-width: 768px) {
            #skip-list h3 {
                font-size: 18px;
            }
        }
        
        #skip-items {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 10px;
        }
        
        @media (max-width: 600px) {
            #skip-items {
                grid-template-columns: 1fr;
            }
        }
        
        #footer {
            margin-top: 30px;
            padding: 20px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            font-size: 14px;
            color: #666;
            line-height: 1.6;
        }
        
        @media (max-width: 768px) {
            #footer {
                margin-top: 20px;
                padding: 15px;
                font-size: 13px;
                border-radius: 6px;
            }
        }
        
        #footer p {
            margin: 0 0 10px 0;
        }
        
        #footer p:last-child {
            margin-bottom: 0;
        }
        
        #footer a {
            color: #0074A2;
            text-decoration: none;
        }
        
        #footer a:hover {
            text-decoration: underline;
        }
        
        .skip-item {
            padding: 15px;
            border-left: 4px solid #e0e0e0;
            background: #f9f9f9;
            border-radius: 4px;
            break-inside: avoid;
            cursor: pointer;
            transition: all 0.2s ease;
            -webkit-tap-highlight-color: rgba(0, 0, 0, 0.05);
            min-height: 48px; /* Touch-friendly */
        }
        
        @media (max-width: 768px) {
            .skip-item {
                padding: 12px;
            }
        }
        
        .skip-item:hover {
            background: #f0f0f0;
            border-left-color: #0074A2;
            transform: translateX(2px);
        }
        
        @media (hover: none) {
            .skip-item:hover {
                background: #f9f9f9;
                transform: none;
            }
            
            .skip-item:active {
                background: #f0f0f0;
                border-left-color: #0074A2;
            }
        }
        
        .skip-item.nearest {
            border-left-color: #FF7043;
            background: #FFF3E0;
        }
        
        .skip-item h4 {
            margin: 0 0 8px 0;
            color: #333;
            font-size: 16px;
        }
        
        @media (max-width: 768px) {
            .skip-item h4 {
                font-size: 15px;
                margin: 0 0 6px 0;
            }
        }
        
        .skip-item p {
            margin: 4px 0;
            font-size: 14px;
            color: #666;
        }
        
        @media (max-width: 768px) {
            .skip-item p {
                font-size: 13px;
            }
        }
        
        .nearest-skip {
            background: #E8F5F9;
            border-left: 4px solid #0074A2;
            padding: 15px;
            margin-bottom: 15px;
            border-radius: 4px;
        }
        
        .nearest-skip h3 {
            margin: 0 0 10px 0;
            color: #0074A2;
            font-size: 18px;
        }
        
        .skip-detail {
            margin: 5px 0;
            font-size: 14px;
        }
        
        .skip-detail strong {
            font-weight: 600;
        }
        
        .error {
            background: #FFEBEE;
            color: #C62828;
            padding: 15px;
            border-radius: 4px;
            border-left: 4px solid #C62828;
            margin-bottom: 20px;
        }
        
        #map {
            height: 500px;
            width: 100%;
        }
        
        .leaflet-popup-content {
            margin: 12px;
            font-size: 14px;
        }
        
        .leaflet-popup-content h4 {
            margin: 0 0 8px 0;
            color: #0074A2;
        }
        
        .emoji {
            font-size: 1.2em;
        }
        
        .skip-count {
            background: #FF7043;
            color: white;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
            display: inline-block;
            margin-left: 8px;
        }
        
        .loading {
            text-align: left;
            padding: 20px;
            color: #0074A2;
            font-size: 16px;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <div id="container">
        <div id="header">
            <h1>Where Mega Skip?</h1>
            <div id="subtitle">Find your nearest Wandsworth Mega Skip</div>
        </div>
        
        <div id="date-banner">
            <div id="date-info">
                <h2>Next Mega Skip Day: <span id="next-date">Loading...</span></h2>
                <span class="time-info">Skips open at 9am and close when they're full, or 12 noon - whichever comes first.</span>
            </div>
            <div class="control-group">
                <button id="useLocation" onclick="requestLocation()">
                    Use My Location
                </button>
                <span style="color: #999;">or</span>
                <input type="text" id="address" placeholder="Enter postcode or address" style="flex: 1;">
                <button onclick="searchAddress()">Search</button>
            </div>
        </div>
        
        <div id="map-container">
            <div id="map-loading">
                <div class="loading-spinner">
                    <div class="spinner"></div>
                    <h3>Loading...</h3>
                </div>
            </div>
            <div id="map"></div>
        </div>
        
        <div id="nearest-info">
            <h3>🎯 Your Nearest Megaskip</h3>
            <div id="nearest-details"></div>
        </div>
        
        <div id="skip-list">
            <h3>All Mega Skip Locations</h3>
            <div id="skip-items">
                <div class="loading">Loading...</div>
            </div>
        </div>
        
        <div id="footer">
            <p> See <a href="https://www.wandsworth.gov.uk/mega-skip-days" target="_blank" rel="noopener noreferrer">Wandsworth Council Mega Skip Days</a> for official information concering mega skip days and locations. </p>
            <p> This page is provided on a best-effort basis to help make it easier to find your nearest Mega Skip. This page is not affiliated with Wandsworth Council in any way.</p>
        </div>
    </div>
    
    <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
    <script>
        const skipLocations = {{.Locations}};
        let map, userMarker, markers = [];
        let userLocation = null;
        let nearestSkipIndex = null;
        let geocodedSkips = [];
        let routeLine = null;
        
        // Initialize map centered on Wandsworth
        function initMap() {
            map = L.map('map').setView([51.4567, -0.1910], 13);
            L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
                attribution: '© OpenStreetMap contributors',
                maxZoom: 19
            }).addTo(map);
            
            // Geocode all skips then add markers
            geocodeAllSkips();
        }
        
        async function geocodeAllSkips() {
            showLoading();
            disableControls();
            
            // Geocode in parallel batches of 3 for faster loading
            const batchSize = 3;
            for (let i = 0; i < skipLocations.length; i += batchSize) {
                const batch = skipLocations.slice(i, i + batchSize);
                const results = await Promise.all(
                    batch.map(async (skip) => {
                        try {
                            const result = await geocodePostcode(skip.postcode);
                            if (result) {
                                return {
                                    ...skip,
                                    lat: result.lat,
                                    lng: result.lng
                                };
                            }
                        } catch (err) {
                            console.error('Failed to geocode', skip.postcode, err);
                        }
                        return null;
                    })
                );
                
                // Add successful results
                results.forEach(result => {
                    if (result) geocodedSkips.push(result);
                });
                
                // Wait between batches to respect rate limits
                if (i + batchSize < skipLocations.length) {
                    await new Promise(resolve => setTimeout(resolve, 500));
                }
            }
            
            addSkipMarkers();
            renderSkipList();
            enableControls();
            hideMapLoading();
            fitMapToSkips();
            
            // Set the date
            if (geocodedSkips.length > 0) {
                document.getElementById('next-date').textContent = geocodedSkips[0].dateStr;
            }
        }
        
        function hideMapLoading() {
            document.getElementById('map-loading').classList.add('hidden');
        }
        
        function fitMapToSkips() {
            if (geocodedSkips.length === 0) return;
            
            // Create bounds that include all skip markers
            const bounds = L.latLngBounds(geocodedSkips.map(skip => [skip.lat, skip.lng]));
            map.fitBounds(bounds, { padding: [50, 50] });
        }
        
        function disableControls() {
            document.getElementById('date-banner').classList.add('disabled');
        }
        
        function enableControls() {
            document.getElementById('date-banner').classList.remove('disabled');
        }
        
        function showLoading() {
            document.getElementById('skip-items').innerHTML = '<div class="loading">Loading...</div>';
        }
        
        function toTitleCase(str) {
            return str.toLowerCase().split(' ').map(function(word) {
                return word.charAt(0).toUpperCase() + word.slice(1);
            }).join(' ');
        }
        
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        function renderSkipList() {
            const container = document.getElementById('skip-items');
            if (geocodedSkips.length === 0) {
                container.innerHTML = '<p style="text-align: center; color: #999;">No upcoming skip days found.</p>';
                return;
            }
            
            let html = '';
            geocodedSkips.forEach(function(skip, index) {
                html += '<div class="skip-item" data-skip-index="' + index + '" onclick="focusSkip(' + index + ')">' +
                    '<h4>📍 ' + escapeHtml(toTitleCase(skip.address)) + '</h4>' +
                    '<p>📮 ' + escapeHtml(skip.postcode) + '</p>' +
                    '<p>📅 ' + escapeHtml(skip.dateStr) + '</p>' +
                    '</div>';
            });
            container.innerHTML = html;
        }
        
        async function geocodePostcode(postcode) {
            const url = 'https://nominatim.openstreetmap.org/search?q=' + 
                encodeURIComponent(postcode + ' London UK') + 
                '&format=json&limit=1&countrycodes=gb';
            
            const response = await fetch(url, {
                headers: { 'User-Agent': 'WhereMegaSkip/1.0 (https://github.com/JosephSalisbury/wheremegaskip)' }
            });
            
            const results = await response.json();
            if (results.length === 0) return null;
            
            return {
                lat: parseFloat(results[0].lat),
                lng: parseFloat(results[0].lon)
            };
        }
        
        function addSkipMarkers() {
            geocodedSkips.forEach(function(skip) {
                if (!skip.lat || !skip.lng) return; // Skip if not geocoded
                
                const marker = L.marker([skip.lat, skip.lng], {
                    icon: L.icon({
                        iconUrl: 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" width="30" height="40" viewBox="0 0 30 40"><path fill="%230074A2" d="M15 0C8.4 0 3 5.4 3 12c0 8.3 12 28 12 28s12-19.7 12-28c0-6.6-5.4-12-12-12z"/><circle cx="15" cy="12" r="5" fill="white"/></svg>'),
                        iconSize: [30, 40],
                        iconAnchor: [15, 40],
                        popupAnchor: [0, -40]
                    })
                });
                
                marker.bindPopup('<h4>' + skip.address + '</h4>' +
                    '<p><strong>📅 ' + skip.dateStr + '</strong></p>' +
                    '<p>🕘 Opens 9am - 12pm (or when full)</p>' +
                    '<p>📮 ' + skip.postcode + '</p>');
                    
                marker.addTo(map);
                marker.skipData = skip;
                markers.push(marker);
            });
        }
        
        function requestLocation() {
            const btn = document.getElementById('useLocation');
            btn.disabled = true;
            btn.textContent = '⏳ Getting location...';
            
            if (!navigator.geolocation) {
                alert('Geolocation is not supported by your browser');
                btn.disabled = false;
                btn.innerHTML = '<span class="emoji">📍</span> Use My Location';
                return;
            }
            
            navigator.geolocation.getCurrentPosition(
                function(position) {
                    userLocation = {
                        lat: position.coords.latitude,
                        lng: position.coords.longitude
                    };
                    updateWithUserLocation();
                    btn.disabled = false;
                    btn.innerHTML = '<span class="emoji">✓</span> Location Set';
                },
                function(error) {
                    let message = 'Unable to get your location';
                    if (error.code === error.PERMISSION_DENIED) {
                        message = 'Location permission denied. Please enable location access or use address search.';
                    }
                    alert(message);
                    btn.disabled = false;
                    btn.innerHTML = '<span class="emoji">📍</span> Use My Location';
                }
            );
        }
        
        function searchAddress() {
            const address = document.getElementById('address').value;
            if (!address) return;
            
            const btn = event.target;
            btn.disabled = true;
            btn.textContent = '🔍 Searching...';
            
            // Use Nominatim to geocode the address
            fetch('https://nominatim.openstreetmap.org/search?q=' + encodeURIComponent(address + ' London UK') + '&format=json&limit=1', {
                headers: { 'User-Agent': 'WhereMegaSkip/1.0 (https://github.com/JosephSalisbury/wheremegaskip)' }
            })
            .then(response => response.json())
            .then(results => {
                if (results.length === 0) {
                    alert('Address not found. Try a different format or postcode.');
                    btn.disabled = false;
                    btn.textContent = 'Search';
                    return;
                }
                userLocation = {
                    lat: parseFloat(results[0].lat),
                    lng: parseFloat(results[0].lon)
                };
                updateWithUserLocation();
                btn.disabled = false;
                btn.textContent = 'Search';
            })
            .catch(error => {
                alert('Failed to search address. Please try again.');
                btn.disabled = false;
                btn.textContent = 'Search';
            });
        }
        
        function updateWithUserLocation() {
            // Add/update user marker
            if (userMarker) {
                map.removeLayer(userMarker);
            }
            
            userMarker = L.marker([userLocation.lat, userLocation.lng], {
                icon: L.icon({
                    iconUrl: 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32"><circle cx="16" cy="16" r="14" fill="%23FF7043" stroke="white" stroke-width="4"/><circle cx="16" cy="16" r="6" fill="white"/></svg>'),
                    iconSize: [32, 32],
                    iconAnchor: [16, 16]
                })
            }).bindPopup('📍 You are here').addTo(map);
            
            // Calculate distances and find nearest
            let nearest = null;
            let nearestDist = Infinity;
            
            geocodedSkips.forEach(function(skip) {
                if (!skip.lat || !skip.lng) return;
                const dist = calculateDistance(userLocation.lat, userLocation.lng, skip.lat, skip.lng);
                skip.distance = dist;
                if (dist < nearestDist) {
                    nearestDist = dist;
                    nearest = skip;
                }
            });
            
            if (nearest) {
                showNearestSkip(nearest);
                
                // Draw line from user to nearest skip
                if (routeLine) {
                    map.removeLayer(routeLine);
                }
                routeLine = L.polyline([
                    [userLocation.lat, userLocation.lng],
                    [nearest.lat, nearest.lng]
                ], {
                    color: '#FF7043',
                    weight: 3,
                    opacity: 0.7,
                    dashArray: '10, 10'
                }).addTo(map);
                
                // Zoom to show both user and nearest skip
                const bounds = L.latLngBounds([
                    [userLocation.lat, userLocation.lng],
                    [nearest.lat, nearest.lng]
                ]);
                map.fitBounds(bounds, { padding: [50, 50] });
                
                // Highlight nearest marker
                highlightNearest(nearest);
            }
        }
        
        function highlightNearest(nearest) {
            markers.forEach(function(marker) {
                if (marker.skipData === nearest) {
                    marker.setIcon(L.icon({
                        iconUrl: 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" width="36" height="48" viewBox="0 0 30 40"><path fill="%23FF7043" d="M15 0C8.4 0 3 5.4 3 12c0 8.3 12 28 12 28s12-19.7 12-28c0-6.6-5.4-12-12-12z"/><circle cx="15" cy="12" r="5" fill="white"/></svg>'),
                        iconSize: [36, 48],
                        iconAnchor: [18, 48],
                        popupAnchor: [0, -48]
                    }));
                }
            });
        }
        
        function calculateDistance(lat1, lon1, lat2, lon2) {
            // Haversine formula
            const R = 6371; // km
            const dLat = (lat2 - lat1) * Math.PI / 180;
            const dLon = (lon2 - lon1) * Math.PI / 180;
            const a = Math.sin(dLat/2) * Math.sin(dLat/2) +
                    Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
                    Math.sin(dLon/2) * Math.sin(dLon/2);
            const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
            return R * c;
        }
        
        function showNearestSkip(skip) {
            // Find and store the index of the nearest skip
            nearestSkipIndex = geocodedSkips.indexOf(skip);
            
            // Show nearest info section
            const nearestInfo = document.getElementById('nearest-info');
            const nearestDetails = document.getElementById('nearest-details');
            
            // Add click handler to nearest info
            nearestInfo.onclick = function() {
                if (nearestSkipIndex !== null) {
                    focusSkip(nearestSkipIndex);
                }
            };
            
            nearestDetails.innerHTML = 
                '<div class="nearest-detail"><strong>📍 Location:</strong> ' + escapeHtml(skip.address) + '</div>' +
                '<div class="nearest-detail"><strong>📮 Postcode:</strong> ' + escapeHtml(skip.postcode) + '</div>' +
                '<div class="nearest-detail"><strong>📅 Available on:</strong> ' + escapeHtml(skip.dateStr) + '</div>';
            
            nearestInfo.classList.add('visible');
            
            // Re-render list with nearest highlighted
            const container = document.getElementById('skip-items');
            let html = '';
            geocodedSkips.forEach(function(s, index) {
                const isNearest = s === skip;
                html += '<div class="skip-item' + (isNearest ? ' nearest' : '') + '" data-skip-index="' + index + '" onclick="focusSkip(' + index + ')">' +
                    '<h4>' + (isNearest ? '🎯 ' : '📍 ') + escapeHtml(toTitleCase(s.address)) + '</h4>' +
                    '<p>📮 ' + escapeHtml(s.postcode) + '</p>' +
                    '<p>📅 ' + escapeHtml(s.dateStr) + '</p>' +
                    '</div>';
            });
            container.innerHTML = html;
            
            // Scroll to nearest info
            nearestInfo.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        }
        
        function focusSkip(index) {
            const skip = geocodedSkips[index];
            const marker = markers[index];
            
            if (skip && marker) {
                // If user location exists, fit bounds to show both
                if (userLocation) {
                    const bounds = L.latLngBounds(
                        [userLocation.lat, userLocation.lng],
                        [skip.lat, skip.lng]
                    );
                    map.fitBounds(bounds, {
                        padding: [50, 50],
                        animate: true,
                        duration: 0.5
                    });
                } else {
                    // No user location, just pan to marker and zoom
                    map.setView([skip.lat, skip.lng], 15, {
                        animate: true,
                        duration: 0.5
                    });
                }
                
                // Open popup
                marker.openPopup();
            }
        }
        
        // Initialize on load
        initMap();
        
        // Allow Enter key in address field
        document.getElementById('address').addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                searchAddress();
            }
        });
    </script>
</body>
</html>`
