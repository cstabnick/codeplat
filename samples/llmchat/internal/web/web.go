// Package web serves the chat page and its script.
package web

import (
	"embed"
	"net/http"
)

//go:embed page.html app.js
var files embed.FS

// NewHandler serves the page: GET /{$} returns page.html, GET /app.js
// returns app.js with Content-Type "text/javascript; charset=utf-8".
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serve("page.html", "text/html; charset=utf-8"))
	mux.HandleFunc("GET /app.js", serve("app.js", "text/javascript; charset=utf-8"))
	return mux
}

// serve returns a handler that writes the embedded file name.
func serve(name, contentType string) http.HandlerFunc {
	body, err := files.ReadFile(name)
	if err != nil {
		panic(err) // the file is embedded, so this cannot happen
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
