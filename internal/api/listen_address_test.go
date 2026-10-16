package api

import "testing"

func TestValidateListenAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		authEnabled bool
		allowRemote bool
		wantErr     bool
	}{
		{name: "ipv4 loopback", addr: "127.0.0.1:5522"},
		{name: "ipv6 loopback", addr: "[::1]:5522"},
		{name: "localhost", addr: "localhost:5522"},
		{name: "empty host", addr: ":5522", wantErr: true},
		{name: "ipv4 wildcard", addr: "0.0.0.0:5522", wantErr: true},
		{name: "ipv6 wildcard", addr: "[::]:5522", wantErr: true},
		{name: "lan address", addr: "192.168.1.20:5522", wantErr: true},
		{name: "hostname", addr: "plex.local:5522", wantErr: true},
		{name: "authenticated remote", addr: ":5522", authEnabled: true},
		{name: "explicit remote override", addr: ":5522", allowRemote: true},
		{name: "malformed", addr: "not-a-listen-address", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListenAddress(tt.addr, tt.authEnabled, tt.allowRemote)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateListenAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}
