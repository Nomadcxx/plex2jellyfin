package jellyfin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyUnknownSeasonEpisodes(t *testing.T) {
	t.Parallel()

	ep := func(path string, season, index *int) Item {
		return Item{Type: "Episode", Path: path, ParentIndexNumber: season, IndexNumber: index}
	}
	n := func(v int) *int { return &v }

	tests := []struct {
		name string
		in   []Item
		want UnknownSeasonClass
	}{
		{name: "empty", want: UnknownSeasonEmpty},
		{
			name: "already indexed virtual season",
			in:   []Item{ep("/tv/Show/Season 01/Show S01E01.mkv", n(1), n(1))},
			want: UnknownSeasonIndexed,
		},
		{
			name: "safe metadata refresh when every episode has sxxeyy",
			in: []Item{
				ep("/tv/Show/Season 01/Show S01E01.mkv", nil, nil),
				ep("/tv/Show/Season 01/Show.S01E02.mkv", nil, nil),
			},
			want: UnknownSeasonRefreshRepairable,
		},
		{
			name: "folder context only is not refresh repairable",
			in: []Item{
				ep("/tv/Show/Season 01/obfuscated-a.mkv", nil, nil),
				ep("/tv/Show/Season 01/obfuscated-b.mkv", nil, nil),
			},
			want: UnknownSeasonFolderContext,
		},
		{
			name: "mixed sxxeyy and obfuscated stays review",
			in: []Item{
				ep("/tv/Show/Season 01/Show S01E01.mkv", nil, nil),
				ep("/tv/Show/Season 01/obfuscated-b.mkv", nil, nil),
			},
			want: UnknownSeasonMixed,
		},
		{
			name: "manual unknown has no season or episode evidence",
			in:   []Item{ep("/tv/Show/random.mkv", nil, nil)},
			want: UnknownSeasonManual,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyUnknownSeasonEpisodes(tt.in)
			assert.Equal(t, tt.want, got.Class)
		})
	}
}

func TestClassifyUnknownSeasonEpisodesCountsRandomishBasenames(t *testing.T) {
	t.Parallel()

	got := ClassifyUnknownSeasonEpisodes([]Item{
		{Type: "Episode", Path: "/tv/Show/Season 01/a1b2c3d4e5f67890.mkv"},
		{Type: "Episode", Path: "/tv/Show/Season 01/Show S01E02.mkv"},
	})

	assert.Equal(t, 1, got.RandomishBasenameCount)
}

func TestAuditUnknownSeasonsUsesUserScopedChildren(t *testing.T) {
	t.Parallel()

	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/Users":
			_, _ = w.Write([]byte(`[
				{"Id":"u1","Name":"viewer","Policy":{"IsAdministrator":false}},
				{"Id":"admin","Name":"admin","Policy":{"IsAdministrator":true}}
			]`))
		case r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("IncludeItemTypes") == "Season":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"season-1","Name":"Season Unknown","SeriesId":"series-1","SeriesName":"Clear Show"},
				{"Id":"season-2","Name":"Season Unknown","SeriesId":"series-2","SeriesName":"Obfuscated Show"}
			],"TotalRecordCount":3}`))
		case r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("ParentId") == "season-1":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"ep-1","Type":"Episode","Path":"/tv/Clear Show/Season 01/Clear Show S01E01.mkv","SeriesId":"series-1"},
				{"Id":"ep-2","Type":"Episode","Path":"/tv/Clear Show/Season 01/Clear Show S01E02.mkv","SeriesId":"series-1"}
			],"TotalRecordCount":2}`))
		case r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("ParentId") == "season-2":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"ep-3","Type":"Episode","Path":"/tv/Obfuscated Show/Season 01/a1b2c3.mkv","SeriesId":"series-2"}
			],"TotalRecordCount":1}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test"})
	report, err := client.AuditUnknownSeasons(context.Background(), "")

	require.NoError(t, err)
	require.Len(t, report.Issues, 2)
	byName := map[string]UnknownSeasonClass{}
	for _, issue := range report.Issues {
		byName[issue.SeriesName] = issue.Class
	}
	assert.Equal(t, UnknownSeasonRefreshRepairable, byName["Clear Show"])
	assert.Equal(t, UnknownSeasonFolderContext, byName["Obfuscated Show"])
	assert.Equal(t, 1, report.RefreshRepairableSeasons)
	assert.Equal(t, 2, report.RefreshRepairableEpisodes)
	assert.Equal(t, 1, report.RefreshCandidateSeasons)
	assert.Equal(t, 2, report.RefreshCandidateEpisodes)
	assert.True(t, strings.HasPrefix(requested[1], "/Users/admin/Items?"), "expected admin user scoped query")
}

func TestRepairUnknownSeasonsRefreshesSeriesWithAnySxxEyyEvidence(t *testing.T) {
	t.Parallel()

	var refreshed []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/Users":
			_, _ = w.Write([]byte(`[{"Id":"admin","Name":"admin","Policy":{"IsAdministrator":true}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("IncludeItemTypes") == "Season":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"season-1","Name":"Season Unknown","SeriesId":"series-1","SeriesName":"Clear Show"},
				{"Id":"season-2","Name":"Season Unknown","SeriesId":"series-2","SeriesName":"Obfuscated Show"},
				{"Id":"season-3","Name":"Season Unknown","SeriesId":"series-3","SeriesName":"Mixed Show"}
			],"TotalRecordCount":2}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("ParentId") == "season-1":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"ep-1","Type":"Episode","Path":"/tv/Clear Show/Season 01/Clear Show S01E01.mkv","SeriesId":"series-1"}],"TotalRecordCount":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("ParentId") == "season-2":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"ep-2","Type":"Episode","Path":"/tv/Obfuscated Show/Season 01/random.mkv","SeriesId":"series-2"}],"TotalRecordCount":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Users/admin/Items" && r.URL.Query().Get("ParentId") == "season-3":
			_, _ = w.Write([]byte(`{"Items":[
				{"Id":"ep-3","Type":"Episode","Path":"/tv/Mixed Show/Season 01/Mixed Show S01E01.mkv","SeriesId":"series-3"},
				{"Id":"ep-4","Type":"Episode","Path":"/tv/Mixed Show/Season 01/random.mkv","SeriesId":"series-3"}
			],"TotalRecordCount":2}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Items/series-1/Refresh":
			refreshed = append(refreshed, r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/Items/series-3/Refresh":
			refreshed = append(refreshed, r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test"})
	report, err := client.RepairUnknownSeasons(context.Background(), "", 10, false)

	require.NoError(t, err)
	assert.Equal(t, 2, report.Audit.RefreshCandidateSeasons)
	assert.Equal(t, 2, report.Audit.RefreshCandidateEpisodes)
	assert.Equal(t, 2, report.Refreshed)
	assert.Equal(t, 1, report.Skipped)
	require.Len(t, refreshed, 2)
	for _, rawQuery := range refreshed {
		assert.Contains(t, rawQuery, "Recursive=true")
		assert.Contains(t, rawQuery, "ReplaceAllImages=false")
	}
}
