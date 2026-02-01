package main

import (
	"log"
	"net/http"
	"os"

	"github.com/JosephSalisbury/wheremegaskip/app"
)

func main() {
	app.InitCache()

	http.HandleFunc("/", app.HandleIndex)
	http.HandleFunc("/api/skips", app.HandleSkipsAPI)
	http.HandleFunc("/calendar.ics", app.HandleCalendarDefault)
	http.HandleFunc("/calendar/", app.HandleCalendarPostcode)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
