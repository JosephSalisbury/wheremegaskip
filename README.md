# Where's My Megaskip? 🚛

A fun web app to help Wandsworth residents find their nearest megaskip location.

## What is this?

Wandsworth Council runs "Mega Skip Days" - free bulk waste disposal events where large skips are placed around the borough. This app helps you:

- 🗺️ See all upcoming megaskip locations on a map
- 📍 Find the nearest skip to your location
- 📏 See distances and directions
- 📅 View dates and times

## Features

- **Live Data**: Scrapes the [Wandsworth Council website](https://www.wandsworth.gov.uk/mega-skip-days) for up-to-date skip locations
- **Smart Caching**: Caches data to avoid hammering the council website
- **Privacy-First**: User location never leaves your browser
- **Geocoding**: Automatically converts postcodes to coordinates
- **Interactive Map**: Uses OpenStreetMap with Leaflet
- **Responsive Design**: Works on desktop and mobile
- **Fun**: Because skip-finding should be enjoyable! 🎉

## Tech Stack

- **Backend**: Go (Golang)
  - HTTP server with `net/http`
  - HTML parsing with `goquery`
  - Nominatim API for geocoding
- **Frontend**: Vanilla JavaScript (no frameworks!)
  - Leaflet.js for maps
  - Geolocation API
  - Client-side distance calculation
- **Styling**: Inspired by Wandsworth Council's design

## Running Locally

```bash
# Install dependencies
go mod download

# Run the server
go run main.go

# Or build and run
go build
./wheremegaskip
```

The server will start on port 8080 (or `$PORT` if set).

## Configuration

- **Cache TTL**: Set `CACHE_TTL_MINUTES` environment variable (default: 60 minutes)
- **Port**: Set `PORT` environment variable (default: 8080)

```bash
CACHE_TTL_MINUTES=30 PORT=3000 go run main.go
```

## Deploying to Vercel

This app is designed to work with Vercel's Go runtime:

1. Connect your GitHub repo to Vercel
2. Vercel will auto-detect the Go project
3. Deploy!

No additional configuration needed.

## How It Works

1. **Server fetches** the council website HTML
2. **Parser extracts** dates, locations, and postcodes  
3. **Geocoder converts** postcodes to lat/lng coordinates
4. **Cache stores** the data for 1 hour (configurable)
5. **Server renders** a single HTML page with data embedded as JSON
6. **Client-side JS** handles:
   - User geolocation
   - Address search
   - Distance calculation (Haversine formula)
   - Map rendering and markers

## Privacy

- Your location is never sent to the server
- All distance calculations happen in your browser
- No tracking, no cookies, no analytics

## Development

See [SPEC.md](SPEC.md) for the full specification and architecture decisions.

## Contributing

Pull requests welcome! Some ideas:

- [ ] Better date parsing (handle more formats)
- [ ] Directions to skip location
- [ ] Notifications for upcoming skips
- [ ] List view option
- [ ] Dark mode

## License

MIT

## Disclaimer

This is an unofficial community project. For official information, visit [wandsworth.gov.uk](https://www.wandsworth.gov.uk/mega-skip-days).
