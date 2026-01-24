package handler

import (
	"net/http"

	"github.com/JosephSalisbury/wheremegaskip/app"
)

// Handler is the Vercel serverless function entry point
func Handler(w http.ResponseWriter, r *http.Request) {
	app.InitCache()
	app.HandleIndex(w, r)
}
