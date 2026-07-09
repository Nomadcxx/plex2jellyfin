package sonarr

import "testing"

func TestJoinURLPreservesQuery(t *testing.T) {
	cases := []struct{ base, ep, want string }{
		{"http://localhost:8989", "/api/v3/queue?page=1&pageSize=100", "http://localhost:8989/api/v3/queue?page=1&pageSize=100"},
		{"http://localhost:8989/", "/api/v3/system/status", "http://localhost:8989/api/v3/system/status"},
		{"http://localhost:8989/sonarr", "/api/v3/queue?page=1", "http://localhost:8989/api/v3/queue?page=1"},
	}
	for _, c := range cases {
		got, err := joinURL(c.base, c.ep)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != c.want {
			t.Errorf("base=%s ep=%s got=%s want=%s", c.base, c.ep, got, c.want)
		}
	}
}
