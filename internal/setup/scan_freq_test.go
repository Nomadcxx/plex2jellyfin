package setup

import "testing"

func TestNormalizeScanFrequency(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"5m", "5m", false},
		{"10", "10m", false},
		{" 15 ", "15m", false},
		{"1h", "1h", false},
		{"0", "", true},
		{"-5", "", true},
		{"nope", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeScanFrequency(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}
