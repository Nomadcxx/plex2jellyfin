package notify

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestJellyfinNotifierName(t *testing.T) {
	n := NewJellyfinNotifier(nil, true)
	if n.Name() != "jellyfin" {
		t.Fatalf("expected name jellyfin, got %s", n.Name())
	}
}

func TestJellyfinNotifierEnabled(t *testing.T) {
	client := jellyfin.NewClient(jellyfin.Config{URL: "http://jellyfin.local", APIKey: "x"})

	if !NewJellyfinNotifier(client, true).Enabled() {
		t.Fatal("expected enabled notifier")
	}
	if NewJellyfinNotifier(client, false).Enabled() {
		t.Fatal("expected disabled notifier")
	}
	if NewJellyfinNotifier(nil, true).Enabled() {
		t.Fatal("expected disabled notifier for nil client")
	}
}

func TestJellyfinNotifierNotify(t *testing.T) {
	calls := 0
	client := jellyfin.NewClient(jellyfin.Config{
		URL:    "http://jellyfin.local",
		APIKey: "k",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/Library/Refresh" {
					calls++
					return jsonResponse(http.StatusNoContent, ``), nil
				}
				if r.URL.Path == "/System/Info" {
					return jsonResponse(http.StatusOK, `{"ServerName":"Jellyfin","Version":"10.10.0"}`), nil
				}
				return jsonResponse(http.StatusNotFound, `not found`), nil
			}),
			Timeout: 2 * time.Second,
		},
	})

	n := NewJellyfinNotifier(client, true)

	movieResult := n.Notify(OrganizationEvent{MediaType: MediaTypeMovie})
	if !movieResult.Success {
		t.Fatalf("expected success for movie notification: %v", movieResult.Error)
	}

	tvResult := n.Notify(OrganizationEvent{MediaType: MediaTypeTVEpisode})
	if !tvResult.Success {
		t.Fatalf("expected success for tv notification: %v", tvResult.Error)
	}

	if calls != 2 {
		t.Fatalf("expected 2 refresh calls, got %d", calls)
	}
}

func TestJellyfinNotifierPing(t *testing.T) {
	client := jellyfin.NewClient(jellyfin.Config{
		URL:    "http://jellyfin.local",
		APIKey: "k",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/System/Info" {
					return jsonResponse(http.StatusOK, `{"ServerName":"Jellyfin","Version":"10.10.0"}`), nil
				}
				return jsonResponse(http.StatusNotFound, `not found`), nil
			}),
			Timeout: 2 * time.Second,
		},
	})

	n := NewJellyfinNotifier(client, true)
	if err := n.Ping(); err != nil {
		t.Fatalf("expected ping success, got %v", err)
	}
}
