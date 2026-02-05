package api

import (
	"io/fs"
	"net/http"
	"path"
)

// spaFileServer serves static files with SPA fallback to index.html
func spaFileServer(root fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent directory traversal
		cleanPath := path.Clean(r.URL.Path)
		if cleanPath == "." {
			cleanPath = "/"
		}

		// Try to open the file
		file, err := root.Open(cleanPath)
		if err != nil {
			// File doesn't exist, serve index.html for SPA routing
			serveIndexHTML(root, w, r)
			return
		}
		defer file.Close()

		// Check if it's a directory
		stat, err := file.Stat()
		if err != nil {
			serveIndexHTML(root, w, r)
			return
		}

		if stat.IsDir() {
			// Try to serve index.html from the directory
			indexPath := path.Join(cleanPath, "index.html")
			indexFile, err := root.Open(indexPath)
			if err != nil {
				serveIndexHTML(root, w, r)
				return
			}
			indexFile.Close()
			http.ServeFileFS(w, r, root, indexPath)
			return
		}

		// Serve the file
		http.ServeFileFS(w, r, root, cleanPath)
	}
}

// serveIndexHTML serves the index.html file for SPA routing
func serveIndexHTML(root fs.FS, w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, root, "index.html")
}
