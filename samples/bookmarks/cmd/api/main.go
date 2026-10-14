package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"bookmarks/internal/api"
	"bookmarks/internal/store"
)

func main() {
	addr := os.Getenv("BOOKMARKS_API_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.NewHandler(store.New()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("bookmarks api listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
