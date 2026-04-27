package radarr

import (
	"fmt"
	"net/url"
	"strings"
)

// joinURL joins baseURL and endpoint while preserving any query string.
// See sonarr.joinURL for rationale.
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
