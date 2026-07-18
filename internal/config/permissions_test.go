package config

import (
	"os/user"
	"strconv"
	"testing"
)

func TestPermissionsResolveUserGroup(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Fatal(err)
	}
	wantUID, err := strconv.Atoi(u.Uid)
	if err != nil {
		t.Fatal(err)
	}
	wantGID, err := strconv.Atoi(g.Gid)
	if err != nil {
		t.Fatal(err)
	}

	p := &PermissionsConfig{User: u.Username, Group: g.Name}
	if got, err := p.ResolveUID(); err != nil || got != wantUID {
		t.Fatalf("ResolveUID() = %d, %v; want %d", got, err, wantUID)
	}
	if got, err := p.ResolveGID(); err != nil || got != wantGID {
		t.Fatalf("ResolveGID() = %d, %v; want %d", got, err, wantGID)
	}
}
