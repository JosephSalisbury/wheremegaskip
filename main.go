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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
