package jellyfin

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func newMockClient(fn roundTripFunc) *Client {
	return NewClient(Config{
		URL:    "http://jellyfin.local",
		APIKey: "test-key",
		HTTPClient: &http.Client{
			Transport: fn,
			Timeout:   2 * time.Second,
		},
	})
}

func TestNewClient(t *testing.T) {
	c := NewClient(Config{URL: "http://example.com", APIKey: "abc"})
	if c.baseURL != "http://example.com" {
		t.Fatalf("unexpected baseURL: %s", c.baseURL)
	}
	if c.apiKey != "abc" {
		t.Fatalf("unexpected apiKey: %s", c.apiKey)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected default timeout 30s, got %s", c.httpClient.Timeout)
	}
	if c.deviceID == "" {
		t.Fatal("expected deviceID to be set")
	}
}

func TestAuthHeader(t *testing.T) {
	c := NewClient(Config{URL: "http://example.com", APIKey: "test-key"})
	header := c.authHeader()

	if !strings.Contains(header, `Token="test-key"`) {
		t.Fatalf("auth header missing token: %s", header)
	}
	if !strings.Contains(header, `Client="jellywatch"`) {
		t.Fatalf("auth header missing client: %s", header)
	}
	if !strings.Contains(header, `DeviceId="`) {
		t.Fatalf("auth header missing device id: %s", header)
	}
}

func TestPing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := newMockClient(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/System/Info" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") == "" {
				t.Fatal("expected Authorization header")
			}
			return jsonResponse(http.StatusOK, `{"ServerName":"Jellyfin","Version":"10.10.0"}`), nil
		})

		if err := c.Ping(); err != nil {
			t.Fatalf("ping failed: %v", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		c := newMockClient(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		})

		err := c.Ping()
		if err == nil {
			t.Fatal("expected ping error")
		}
	})
}

func TestRefreshLibrary(t *testing.T) {
	called := false
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/Library/Refresh" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		called = true
		return jsonResponse(http.StatusNoContent, ``), nil
	})

	if err := c.RefreshLibrary(); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if !called {
		t.Fatal("expected refresh call")
	}
}

func TestRefreshItem(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/Items/abc123/Refresh" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "ReplaceAllMetadata") {
			t.Fatalf("expected refresh payload, got: %s", string(body))
		}
		return jsonResponse(http.StatusNoContent, ``), nil
	})

	if err := c.RefreshItem("abc123"); err != nil {
		t.Fatalf("refresh item failed: %v", err)
	}
}

func TestGetVirtualFolders(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/Library/VirtualFolders" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `[{"Name":"Movies","Locations":["/media/movies"],"CollectionType":"movies","ItemId":"vf1"}]`), nil
	})

	folders, err := c.GetVirtualFolders()
	if err != nil {
		t.Fatalf("GetVirtualFolders failed: %v", err)
	}
	if len(folders) != 1 || folders[0].Name != "Movies" {
		t.Fatalf("unexpected folders: %+v", folders)
	}
}

func TestGetSessionsAndIsPathBeingPlayed(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/Sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `[
			{"Id":"1","UserName":"alice","DeviceName":"tv"},
			{"Id":"2","UserName":"bob","DeviceName":"ipad","NowPlayingItem":{"Path":"/media/movies/Inception.mkv"}}
		]`), nil
	})

	sessions, err := c.GetSessions()
	if err != nil {
		t.Fatalf("GetSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	playing, sess, err := c.IsPathBeingPlayed("/media/movies/Inception.mkv")
	if err != nil {
		t.Fatalf("IsPathBeingPlayed failed: %v", err)
	}
	if !playing || sess == nil || sess.UserName != "bob" {
		t.Fatalf("unexpected playing result: playing=%v session=%+v", playing, sess)
	}

	playing, sess, err = c.IsPathBeingPlayed("/media/movies/Other.mkv")
	if err != nil {
		t.Fatalf("IsPathBeingPlayed failed: %v", err)
	}
	if playing || sess != nil {
		t.Fatalf("expected no match, got playing=%v session=%+v", playing, sess)
	}
}

func TestSearchItems(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/Items" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("SearchTerm"); got != "The Matrix" {
			t.Fatalf("unexpected SearchTerm: %q", got)
		}
		if got := r.URL.Query().Get("IncludeItemTypes"); got != "Movie,Series" {
			t.Fatalf("unexpected IncludeItemTypes: %q", got)
		}
		return jsonResponse(http.StatusOK, `{"Items":[{"Id":"m1","Name":"The Matrix"}],"TotalRecordCount":1}`), nil
	})

	resp, err := c.SearchItems("The Matrix", "Movie", "Series")
	if err != nil {
		t.Fatalf("SearchItems failed: %v", err)
	}
	if resp.TotalRecordCount != 1 || len(resp.Items) != 1 || resp.Items[0].ID != "m1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetPublicInfo(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/System/Info/Public" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("did not expect auth header for public info, got: %s", auth)
		}
		return jsonResponse(http.StatusOK, `{"ServerName":"Jellyfin","Version":"10.10.0"}`), nil
	})

	info, err := c.GetPublicInfo()
	if err != nil {
		t.Fatalf("GetPublicInfo failed: %v", err)
	}
	if info.ServerName != "Jellyfin" {
		t.Fatalf("unexpected server name: %s", info.ServerName)
	}
}

func TestGetActiveStreams(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `[
			{"Id":"1","UserName":"idle"},
			{"Id":"2","UserName":"playing","NowPlayingItem":{"Path":"/x.mkv"}}
		]`), nil
	})

	active, err := c.GetActiveStreams()
	if err != nil {
		t.Fatalf("GetActiveStreams failed: %v", err)
	}
	if len(active) != 1 || active[0].UserName != "playing" {
		t.Fatalf("unexpected active streams: %+v", active)
	}
}

func TestGetSessionsError(t *testing.T) {
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `boom`), nil
	})

	_, err := c.GetSessions()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}
