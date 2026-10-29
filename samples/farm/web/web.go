// Package web serves the Gossip Acres page and its scripts.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html js
var files embed.FS

// Handler serves the embedded files: GET / returns index.html; GET /js/<name>.js returns that
// script with Content-Type "text/javascript; charset=utf-8"; anything else that does not exist is
// 404. Every response has "Cache-Control: no-cache".
func Handler() http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		switch {
		case r.URL.Path == "/":
			body, err := files.ReadFile("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/js/") && strings.HasSuffix(r.URL.Path, ".js"):
			// Stat first: the file server strips Cache-Control from its error responses.
			if _, err := fs.Stat(files, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			fileServer.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
