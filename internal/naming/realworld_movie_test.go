package naming

import (
	"testing"
)

func TestRealWorldMovieParsing(t *testing.T) {
	cases := []struct{
		input, wantTitle, wantYear string
	}{
		{"epic.elvis.presley.in.concert.2025.1080p.webrip.x264.aac5.1-yts.bz.mp4", "epic elvis presley in concert", "2025"},
		{"Scream.7.2026.1080p.WEB-DL.EAC3.5.1.Atmos.H.265.mkv", "Scream 7", "2026"},
		{"Avatar.Fire.and.Ash.2025.SDR.1080p.WEB-DL.mkv", "Avatar Fire and Ash", "2025"},
		{"The.Room.Next.Door.2024.6CH.1080p.BluRay.mkv", "The Room Next Door", "2024"},
		{"How.to.Make.A.Killing.2026.1080p.WEB-DL.AAC5.1.mkv", "How to Make A Killing", "2026"},
		{"Pretty.Lethal.2026.AMZN.EAC3.@TSRG.mkv", "Pretty Lethal", "2026"},
		{"Kraven.the.Hunter.2024.1080p.BluRay.mkv", "Kraven the Hunter", "2024"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			info, err := ParseMovieName(c.input)
			if err != nil {
				t.Fatalf("ParseMovieName() error: %v", err)
			}
			t.Logf("title=%q year=%q", info.Title, info.Year)
			if info.Title != c.wantTitle {
				t.Errorf("title = %q, want %q", info.Title, c.wantTitle)
			}
			if info.Year != c.wantYear {
				t.Errorf("year = %q, want %q", info.Year, c.wantYear)
			}
		})
	}
}
