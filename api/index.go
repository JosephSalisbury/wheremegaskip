package handler

import (
	"net/http"
	"strings"

	"github.com/JosephSalisbury/wheremegaskip/app"
)

// Handler is the Vercel serverless function entry point
func Handler(w http.ResponseWriter, r *http.Request) {
	app.InitCache()

	// Route to appropriate handler based on path
	if strings.HasPrefix(r.URL.Path, "/api/skips") {
		app.HandleSkipsAPI(w, r)
		return
	}

	if r.URL.Path == "/calendar.ics" {
		app.HandleCalendarDefault(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/calendar/") && strings.HasSuffix(r.URL.Path, ".ics") {
		app.HandleCalendarPostcode(w, r)
		return
	}

	app.HandleIndex(w, r)
}
