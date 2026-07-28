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

	var calledMediaUpdate bool
	var calledLibraryRefresh bool

	n.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/Library/Media/Updated":
			calledMediaUpdate = true
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
	if !calledMediaUpdate {
		t.Fatal("expected scoped media update")
	}
	if calledLibraryRefresh {
		t.Fatal("did not expect library refresh")
	}
}

func TestJellyfinNotifierNotifiesOnlyCommittedFolder(t *testing.T) {
	n := NewJellyfinNotifier("http://jf.local", "key", true)

	var updatedPath string
	var calledLibraryRefresh bool

	n.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/Items"):
			return jsonResponse(200, `{"Items":[]}`), nil
		case req.Method == http.MethodPost && req.URL.Path == "/Library/Media/Updated":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read media update: %v", err)
			}
			if strings.Contains(string(body), "/library/Movies/Unknown Movie (2025)") {
				updatedPath = "/library/Movies/Unknown Movie (2025)"
			}
			return jsonResponse(204, ``), nil
		case req.Method == http.MethodPost && req.URL.Path == "/Library/Refresh":
			calledLibraryRefresh = true
			return jsonResponse(204, ``), nil
		default:
			return jsonResponse(404, `{}`), nil
		}
	})}

	res := n.Notify(OrganizationEvent{
		MediaType:         MediaTypeMovie,
		Title:             "Unknown Movie",
		TargetPath:        "/mnt/movies/Unknown Movie (2025)/Unknown Movie (2025).mkv",
		JellyfinTargetDir: "/library/Movies/Unknown Movie (2025)",
	})

	if !res.Success {
		t.Fatalf("expected success, got error: %v", res.Error)
	}
	if updatedPath == "" {
		t.Fatal("expected scoped /Library/Media/Updated notification")
	}
	if calledLibraryRefresh {
		t.Fatal("must not trigger a whole-library refresh")
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
