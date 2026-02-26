package notify

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestJellyfinNotifierTargetedRefresh(t *testing.T) {
	n := NewJellyfinNotifier("http://jf.local", "key", true)

	var calledSearch bool
	var calledItemRefresh bool
	var calledLibraryRefresh bool

	n.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/Items"):
			calledSearch = true
			return jsonResponse(200, `{"Items":[{"Id":"it-1","Name":"The Matrix","Path":"/library/Movies/The Matrix (1999)/The Matrix (1999).mkv","ProductionYear":1999}]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/Items/it-1/Refresh":
			calledItemRefresh = true
			return jsonResponse(204, ``), nil
		case req.Method == http.MethodPost && req.URL.Path == "/Library/Refresh":
			calledLibraryRefresh = true
			return jsonResponse(204, ``), nil
		default:
			return jsonResponse(404, `{}`), nil
		}
	})}

	res := n.Notify(OrganizationEvent{
		MediaType:  MediaTypeMovie,
		Title:      "The Matrix",
		Year:       "1999",
		TargetPath: "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv",
	})

	if !res.Success {
		t.Fatalf("expected success, got error: %v", res.Error)
	}
	if !calledSearch || !calledItemRefresh {
		t.Fatalf("expected search and item refresh calls")
	}
	if calledLibraryRefresh {
		t.Fatalf("did not expect library refresh fallback")
	}
}

func TestJellyfinNotifierFallsBackToLibraryRefresh(t *testing.T) {
	n := NewJellyfinNotifier("http://jf.local", "key", true)

	var calledLibraryRefresh bool

	n.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/Items"):
			return jsonResponse(200, `{"Items":[]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/Library/Refresh":
			calledLibraryRefresh = true
			return jsonResponse(204, ``), nil
		default:
			return jsonResponse(404, `{}`), nil
		}
	})}

	res := n.Notify(OrganizationEvent{
		MediaType:  MediaTypeMovie,
		Title:      "Unknown Movie",
		TargetPath: "/library/Movies/Unknown Movie (2025)/Unknown Movie (2025).mkv",
	})

	if !res.Success {
		t.Fatalf("expected success, got error: %v", res.Error)
	}
	if !calledLibraryRefresh {
		t.Fatalf("expected library refresh fallback")
	}
}

func TestJellyfinNotifierPing(t *testing.T) {
	n := NewJellyfinNotifier("http://jf.local", "key", true)
	n.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet && req.URL.Path == "/System/Info" {
			return jsonResponse(200, `{}`), nil
		}
		return jsonResponse(404, `{}`), nil
	})}

	if err := n.Ping(); err != nil {
		t.Fatalf("Ping() failed: %v", err)
	}
}
