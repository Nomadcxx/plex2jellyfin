package plugininstall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeJellyfin serves just enough of the admin API for the engine.
type fakeJellyfin struct {
	version       string
	repos         []map[string]any
	plugins       []map[string]any
	pluginHealthy bool

	repoPosts    [][]map[string]any
	installCalls []string
	restartCalls int
	configPosts  []map[string]any
}

func (f *fakeJellyfin) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /System/Info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"Version": f.version})
	})
	mux.HandleFunc("GET /Repositories", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.repos)
	})
	mux.HandleFunc("POST /Repositories", func(w http.ResponseWriter, r *http.Request) {
		var posted []map[string]any
		json.NewDecoder(r.Body).Decode(&posted)
		f.repoPosts = append(f.repoPosts, posted)
		f.repos = posted
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /Plugins", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.plugins)
	})
	mux.HandleFunc("POST /Packages/Installed/", func(w http.ResponseWriter, r *http.Request) {
		f.installCalls = append(f.installCalls, r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /System/Restart", func(w http.ResponseWriter, r *http.Request) {
		f.restartCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /Plugins/", func(w http.ResponseWriter, r *http.Request) {
		var cfg map[string]any
		json.NewDecoder(r.Body).Decode(&cfg)
		f.configPosts = append(f.configPosts, cfg)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /plex2jellyfin/health", func(w http.ResponseWriter, r *http.Request) {
		if !f.pluginHealthy {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"Status": "healthy"})
	})
	mux.HandleFunc("POST /plex2jellyfin/test-webhook", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Sent": true, "DaemonUrl": "http://10.0.0.5:5522/api/v1/webhooks/jellyfin",
			"DaemonStatusCode": 200, "Authenticated": true,
		})
	})
	return mux
}

func newTestEngine(t *testing.T, f *fakeJellyfin) *Engine {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	e := New(srv.URL, "test-key", srv.Client())
	e.pollInterval = 10 * time.Millisecond
	return e
}

func TestInspectReportsServerAndPluginState(t *testing.T) {
	f := &fakeJellyfin{
		version:       "10.11.6",
		repos:         []map[string]any{{"Name": "Existing", "Url": "https://example.org/m.json", "Enabled": true}},
		plugins:       []map[string]any{{"Id": strings.ReplaceAll(PluginGUID, "-", ""), "Name": "Plex2Jellyfin", "Version": "1.2.0.0"}},
		pluginHealthy: true,
	}
	insp, err := newTestEngine(t, f).Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !insp.ABISupported || insp.ServerVersion != "10.11.6" {
		t.Errorf("ABI gate wrong: %+v", insp)
	}
	if insp.RepoRegistered {
		t.Error("repo should not be registered yet")
	}
	if insp.InstalledVersion != "1.2.0.0" || !insp.PluginResponding {
		t.Errorf("plugin detection wrong: %+v", insp)
	}
}

func TestInspectGatesOldServers(t *testing.T) {
	f := &fakeJellyfin{version: "10.10.7"}
	insp, err := newTestEngine(t, f).Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if insp.ABISupported {
		t.Error("10.10.x must not pass the ABI gate")
	}
}

func TestRegisterRepoPreservesExistingRepositories(t *testing.T) {
	f := &fakeJellyfin{
		version: "10.11.6",
		repos:   []map[string]any{{"Name": "Existing", "Url": "https://example.org/m.json", "Enabled": true}},
	}
	added, err := newTestEngine(t, f).RegisterRepo(context.Background())
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if len(f.repoPosts) != 1 || len(f.repoPosts[0]) != 2 {
		t.Fatalf("expected one POST with 2 repos, got %+v", f.repoPosts)
	}
	if f.repoPosts[0][0]["Url"] != "https://example.org/m.json" {
		t.Error("existing repository was clobbered")
	}
	if f.repoPosts[0][1]["Url"] != ManifestURL {
		t.Error("our manifest URL missing from the posted list")
	}
}

func TestRegisterRepoIsIdempotent(t *testing.T) {
	f := &fakeJellyfin{
		version: "10.11.6",
		repos:   []map[string]any{{"Name": RepoName, "Url": ManifestURL, "Enabled": true}},
	}
	added, err := newTestEngine(t, f).RegisterRepo(context.Background())
	if err != nil || added {
		t.Fatalf("added=%v err=%v; want no-op", added, err)
	}
	if len(f.repoPosts) != 0 {
		t.Error("repository list must not be re-posted when already registered")
	}
}

func TestInstallTargetsThePackageByGUID(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6"}
	if err := newTestEngine(t, f).Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.installCalls) != 1 ||
		!strings.Contains(f.installCalls[0], "/Packages/Installed/Plex2Jellyfin") ||
		!strings.Contains(f.installCalls[0], "assemblyGuid="+PluginGUID) {
		t.Fatalf("install call wrong: %v", f.installCalls)
	}
}

func TestConfigurePushesSecretAndDaemonURL(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6"}
	err := newTestEngine(t, f).Configure(context.Background(), "http://10.0.0.5:5522/", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.configPosts) != 1 {
		t.Fatalf("config posts: %+v", f.configPosts)
	}
	got := f.configPosts[0]
	if got["Plex2JellyfinUrl"] != "http://10.0.0.5:5522" {
		t.Errorf("daemon URL not normalized: %v", got["Plex2JellyfinUrl"])
	}
	if got["SharedSecret"] != "s3cret" || got["EnableEventForwarding"] != true {
		t.Errorf("config body wrong: %+v", got)
	}
}

func TestWaitReadySucceedsOnceHealthy(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: true}
	if err := newTestEngine(t, f).WaitReady(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitReadyTimesOut(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: false}
	err := newTestEngine(t, f).WaitReady(context.Background(), 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestVerifyParsesThePluginResponse(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: true}
	res, err := newTestEngine(t, f).Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Sent || !res.Authenticated || res.DaemonStatusCode != 200 {
		t.Errorf("verify result wrong: %+v", res)
	}
}
