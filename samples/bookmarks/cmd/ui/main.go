package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"bookmarks/internal/client"
	"bookmarks/internal/ui"
)

func main() {
	addr := envOr("BOOKMARKS_UI_ADDR", "127.0.0.1:8082")
	apiURL := envOr("BOOKMARKS_API_URL", "http://127.0.0.1:8081")

	handler := ui.NewHandler(client.New(apiURL, &http.Client{Timeout: 5 * time.Second}))
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("bookmarks ui listening on %s, api at %s", addr, apiURL)
	if err := srv.ListenAndServe(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
