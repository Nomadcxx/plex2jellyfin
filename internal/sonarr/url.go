package sonarr

import (
	"fmt"
	"net/url"
	"strings"
)

// joinURL joins baseURL and endpoint while preserving any query string in
// endpoint. url.JoinPath percent-encodes "?" because it treats the entire
// argument as a path segment, which breaks endpoints like
// "/api/v3/queue?page=1" — Sonarr/Radarr then return 404.
func joinURL(baseURL, endpoint string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	if !strings.HasPrefix(resolved.Path, "/") && resolved.Path != "" {
		resolved.Path = "/" + resolved.Path
	}
	if resolved.Scheme == "" || resolved.Host == "" {
		return "", fmt.Errorf("invalid base URL: %s", baseURL)
	}
	return resolved.String(), nil
}
