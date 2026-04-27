package radarr

import "testing"

func TestJoinURLPreservesQuery(t *testing.T) {
	got, err := joinURL("http://localhost:7878", "/api/v3/queue?page=1&pageSize=100&includeMovie=true")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "http://localhost:7878/api/v3/queue?page=1&pageSize=100&includeMovie=true"
	if got != want {
		t.Errorf("got=%s want=%s", got, want)
	}
}
