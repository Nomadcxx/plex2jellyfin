package api

import (
	"net/http"
	"time"
)

// NewHTTPServer applies the request timeouts shared by the web and CLI API
// listeners. WriteTimeout stays zero because the API serves long-lived SSE.
func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
