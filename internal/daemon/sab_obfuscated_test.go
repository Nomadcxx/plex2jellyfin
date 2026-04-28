package daemon

import "testing"

func TestIsObfuscatedSABFilename(t *testing.T) {
cases := []struct {
path string
want bool
}{
{"/x/SXvWQZqPGRTeZvy6oGudBsA2FUBH1HUd.mkv", true},
{"/x/abcdefghijklmnopqrstu.mp4", true},
{"/x/Show.S01E01.mkv", false},
{"/x/short.mkv", false},
{"/x/SXvWQZqPGRTeZvy6oGudBsA2FUBH1HUd.txt", false},
{"/x/BEEF.S01E02.1080p.WEB.h264-ETHEL/q1reIwWo3oVx97qiPp0731Eglz7WFVn8.mkv", false},
{"/x/Some.Show.S01.Complete/abcdefghijklmnopqrstu.mkv", false},
{"/x/Movie.2024.1080p/abcdefghijklmnopqrstu.mkv", false},
}
for _, c := range cases {
if got := IsObfuscatedSABFilename(c.path); got != c.want {
t.Errorf("%s: got %v, want %v", c.path, got, c.want)
}
}
}
