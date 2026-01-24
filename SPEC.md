# Wandsworth Megaskip Finder - Specification

## Goal
Build a website that helps users find the nearest Wandsworth Council megaskip location based on their current location or a specified address.

## Data Source
- **Primary Source**: https://www.wandsworth.gov.uk/mega-skip-days
- **Approach**: Go server fetches and parses the council website
- **Benefits**: 
  - No CORS issues (server-side fetching)
  - Live data on every page load
  - Can cache parsed data to reduce load on council site
- **Parsing Requirements**:
  - Handle multiple skip days per location robustly
  - Extract all dates, times, and locations from the page
  - Parse dates correctly (various formats possible)
  - Associate dates with locations accurately
  - Handle edge cases (missing data, formatting changes)

## Core Features

### 1. Location Detection
- Automatically detect user's location using browser geolocation API
- Require user opt-in/permission before accessing location
- Fallback to manual address entry if permission denied or unavailable

### 2. Manual Address Entry
- Allow users to manually enter an address or postcode
- Geocode the address to coordinates for distance calculation

### 3. Date Filtering
- Parse all skip days from the council website
- Filter to show only upcoming megaskip days (future dates)
- For locations with multiple upcoming skip days:
  - Display the next upcoming date prominently
  - Optionally show all upcoming dates for that location
- Handle date parsing robustly (council may use various formats)
- Compare dates accurately considering timezones (London time)

### 4. Map Display
- Use OpenStreetMap with Leaflet.js library
- Display all upcoming megaskip locations as markers
- Highlight the nearest skip location distinctly (different color/icon)
- Center map on user's location or entered address

### 5. Information Display
- Show text details for the nearest skip:
  - Address/location name
  - Date and time of the skip day
  - Distance from user's location
- Map markers should be clickable to show skip details

## Technical Requirements

### Technology Stack
- **Backend**: Go (Golang)
  - HTTP server (net/http or similar)
  - HTML parsing library (goquery or similar)
  - Caching to avoid hammering council website
- **Hosting**: Vercel
  - Supports Go via serverless functions
  - May need to consider cold start times
  - Cache is important to minimize function execution time
- **Frontend**: Vanilla JS embedded in single HTML page
- **Mapping**: Leaflet.js with OpenStreetMap tiles (via CDN)
- **Geocoding**: Nominatim (OpenStreetMap's free geocoding service) - server-side

### Browser Support
- Modern browsers with geolocation API support
- Responsive design for mobile and desktop

### Architecture
```
main.go         - Go HTTP server:
                  - Fetches data from council website
                  - Parses HTML to extract megaskip data
                  - Geocodes addresses to lat/lng (or caches coordinates)
                  - Serves index.html with embedded data
                index.html      - Single page with:           - HTML structure
                  - Inline CSS in <style> tags
                  - Inline JavaScript in <script> tags
                  - Data injected from Go server (template or JSON)
```

### Theming
- Inspired by www.wandsworth.gov.uk color scheme and design
- **No branded content**: No logos, council branding, or official imagery
- **Make it fun**: Add playful touches while keeping it functional
  - Friendly copy/messaging
  - Fun icons or animations
  - Encouraging tone

## Error Handling

### User Location Permission Denied
- Display clear message: "Location access denied. Please enter your address manually."
- Show address input form prominently

### Address Lookup Fails
- Display error: "Could not find the address. Please try again with a different format."
- Allow user to retry

### Council Website Down/Unreachable
- Display error: "Unable to fetch megaskip data. Please try again later."
- Consider showing last known data if caching is added later

### No Upcoming Skip Days
- Display message: "No upcoming megaskip days found."

## UI/UX Guidelines

### Layout
- Clean, simple interface
- Map takes primary focus (large viewport area)
- Search/location controls at top
- Nearest skip information prominently displayed (sidebar or top panel)

### Interaction Flow
1. User lands on page (megaskip data already fetched and embedded)
2. Prompt for location permission OR show address input
3. Calculate distances to all skip locations
4. Display map with markers
5. Highlight nearest skip with details

### Visual Design
- Use Wandsworth Council colors if appropriate
- Clear visual distinction between nearest skip and other skips
- Responsive design for mobile use

## Technical Decisions Made
1. **Backend**: Go with net/http for serving and goquery for HTML parsing
2. **Caching**: Cache parsed data for 1+ hour (configurable via environment variable or flag)
3. **Geocoding**: All geocoding happens client-side using Nominatim (both for skip postcodes and user-entered addresses)
4. **User Location**: All user location handling client-side (no user data sent to server)
5. **Distance Calculation**: Haversine formula client-side in JavaScript
6. **Data Delivery**: JSON embedded directly in HTML template
7. **Single Page**: Server renders one HTML page with skip data as inline JSON

## Privacy
- User's location never sent to server
- All distance calculations happen in browser
- Server only provides skip locations with pre-geocoded coordinates

## Implementation Plan
1. Fetch and examine council website HTML structure
2. Create Go server with basic HTTP handler
3. Implement HTML parsing to extract megaskip data (goquery)
4. Geocode addresses to coordinates (cache results)
5. Create single index.html template
6. Theme based on wandsworth.gov.uk color scheme
7. Embed data in HTML (via Go template or inline JSON)
8. Add geolocation detection (client-side JS)
9. Integrate Leaflet map (via CDN)
10. Implement distance calculation (haversine formula)
11. Add nearest skip highlighting
12. Implement manual address entry with geocoding
13. Add error handling (server and client)
14. Add fun/friendly touches to UI
15. Test on mobile and desktop
