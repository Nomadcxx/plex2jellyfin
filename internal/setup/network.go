package setup

import "net"

// DetectAdvertiseIP returns the host's primary outbound IPv4 address,
// or "" when it cannot be determined. Dialing UDP sends no packets -
// it only asks the kernel which source address routes toward a public
// destination (192.0.2.1 is TEST-NET-1, never actually contacted).
func DetectAdvertiseIP() string {
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsLoopback() || addr.IP.IsUnspecified() {
		return ""
	}
	return addr.IP.String()
}
