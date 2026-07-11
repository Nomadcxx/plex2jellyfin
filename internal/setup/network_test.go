package setup

import (
	"net"
	"testing"
)

func TestDetectAdvertiseIPReturnsEmptyOrRoutableIP(t *testing.T) {
	ip := DetectAdvertiseIP()
	if ip == "" {
		t.Skip("no outbound route in this environment")
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("not an IP: %q", ip)
	}
	if parsed.IsLoopback() {
		t.Fatalf("loopback is never a useful advertise address: %q", ip)
	}
}
