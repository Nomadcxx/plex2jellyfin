package setup

import (
	"os"
	"os/user"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

// DefaultPermissions mirrors the TUI installer defaults: empty user (daemon/root
// owns), shared group for Sonarr/Radarr/Jellyfin, group-writable modes.
func DefaultPermissions() config.PermissionsConfig {
	p := config.PermissionsConfig{
		Group:    "media",
		FileMode: "0664",
		DirMode:  "0775",
	}
	if ms := detectMediaServer(); ms != nil {
		p.User = ms.User
		p.Group = ms.Group
		return p
	}
	if g := detectMediaGroup(); g != "" {
		p.Group = g
	}
	return p
}

// ActualUsername is the interactive user (SUDO_USER when escalated), not root.
func ActualUsername() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// UserInGroup reports whether username is a member of groupname.
func UserInGroup(username, groupname string) bool {
	if username == "" || groupname == "" {
		return false
	}
	u, err := user.Lookup(username)
	if err != nil {
		return false
	}
	g, err := user.LookupGroup(groupname)
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, gid := range gids {
		if gid == g.Gid {
			return true
		}
	}
	return false
}

type detectedMediaServer struct {
	Name  string
	User  string
	Group string
}

func detectMediaServer() *detectedMediaServer {
	for _, c := range []detectedMediaServer{
		{Name: "jellyfin", User: "jellyfin", Group: "jellyfin"},
		{Name: "plex", User: "plex", Group: "plex"},
		{Name: "emby", User: "emby", Group: "emby"},
	} {
		if _, err := user.Lookup(c.User); err == nil {
			return &c
		}
	}
	return nil
}

// DetectedMediaServerName returns "jellyfin"/"plex"/"emby" when that system
// user exists, otherwise "".
func DetectedMediaServerName() string {
	if ms := detectMediaServer(); ms != nil {
		return ms.Name
	}
	return ""
}

func detectMediaGroup() string {
	for _, g := range []string{"media", "video", "render"} {
		if _, err := user.LookupGroup(g); err == nil {
			return g
		}
	}
	return ""
}
