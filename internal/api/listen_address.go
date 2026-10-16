package api

import (
	"fmt"
	"net"
	"strings"
)

// ValidateListenAddress rejects unauthenticated HTTP binds that may accept
// remote clients unless the caller explicitly opts into that exposure.
func ValidateListenAddress(addr string, authEnabled, allowUnauthenticatedRemote bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if authEnabled || allowUnauthenticatedRemote {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ipHost, _, hasZone := strings.Cut(host, "%"); hasZone {
		host = ipHost
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing unauthenticated remote HTTP bind %q; configure a password or pass --allow-unauthenticated-remote", addr)
}
