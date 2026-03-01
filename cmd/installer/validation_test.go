package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTestJellyfinCmd_UsesValidAuthorizationHeader(t *testing.T) {
	const apiKey = "abc123"
	wantAuth := `MediaBrowser Token="abc123", Client="jellywatch-installer", Device="installer", DeviceId="installer", Version="1.0.0"`

	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/System/Info" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != wantAuth {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ServerName":"Jellyfin","Version":"10.10.7"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	cmd := testJellyfinCmd("http://jellyfin.local", apiKey)
	msg := cmd()
	got, ok := msg.(apiTestResultMsg)
	if !ok {
		t.Fatalf("expected apiTestResultMsg, got %T", msg)
	}
	if !got.success {
		t.Fatalf("expected success, got error: %v", got.err)
	}
	if got.version != "Jellyfin (10.10.7)" {
		t.Fatalf("unexpected version label: %q", got.version)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
