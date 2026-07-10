package main

import "testing"

func TestResolveHealthAddr(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		flag       string
		changed    bool
		want       string
	}{
		{name: "config overrides flag default", configured: ":18686", flag: ":8686", want: ":18686"},
		{name: "explicit flag overrides config", configured: ":18686", flag: ":28686", changed: true, want: ":28686"},
		{name: "default without config", flag: ":8686", want: ":8686"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveHealthAddr(tt.configured, tt.flag, tt.changed); got != tt.want {
				t.Fatalf("resolveHealthAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}
