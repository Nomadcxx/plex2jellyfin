package main

import (
	"net/http"
	"testing"
	"time"
)

func TestServeHTTPServerConfiguresRequestAndIdleTimeouts(t *testing.T) {
	server := newServeHTTPServer("127.0.0.1:8080", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("read-header timeout = %v, want 10s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 30*time.Second {
		t.Fatalf("read timeout = %v, want 30s", server.ReadTimeout)
	}
	if server.IdleTimeout != 120*time.Second {
		t.Fatalf("idle timeout = %v, want 120s", server.IdleTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("write timeout = %v, want 0 for event streams", server.WriteTimeout)
	}
}
