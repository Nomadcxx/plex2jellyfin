package naming

import (
	"testing"
)

func TestRealWorldMovieParsing(t *testing.T) {
	cases := []struct {
		input, wantTitle, wantYear string
	}{
		{"epic.elvis.presley.in.concert.2025.1080p.webrip.x264.aac5.1-yts.bz.mp4", "epic elvis presley in concert", "2025"},
		{"Scream.7.2026.1080p.WEB-DL.EAC3.5.1.Atmos.H.265.mkv", "Scream 7", "2026"},
		{"Avatar.Fire.and.Ash.2025.SDR.1080p.WEB-DL.mkv", "Avatar Fire and Ash", "2025"},
		{"The.Room.Next.Door.2024.6CH.1080p.BluRay.mkv", "The Room Next Door", "2024"},
		{"How.to.Make.A.Killing.2026.1080p.WEB-DL.AAC5.1.mkv", "How to Make A Killing", "2026"},
		{"Pretty.Lethal.2026.AMZN.EAC3.@TSRG.mkv", "Pretty Lethal", "2026"},
		{"Kraven.the.Hunter.2024.1080p.BluRay.mkv", "Kraven the Hunter", "2024"},
		// Regression: TSRG-style shorthand where the "3" is dropped from
		// EAC3 so the audio tag reads EAC5.1. Without the EAC3? variant
		// in the codec regex, "EAC5 1" survives stripping and lands in
		// the parsed title (Triple.Frontier...EAC5.1-TSRG.mkv -> the
		// movie was imported into Jellyfin as "Triple Frontier EAC5 1").
		{"Triple.Frontier.2019.1080p.NF.WEB-DL.H.264.DUAL.EAC5.1-TSRG.mkv", "Triple Frontier", "2019"},
		{"Some.Movie.2024.1080p.WEB-DL.EAC7.1-GROUP.mkv", "Some Movie", "2024"},
		{"Foo.Bar.2024.1080p.WEB-DL.EAC2.0-GROUP.mkv", "Foo Bar", "2024"},
		{"Baz.Qux.2024.1080p.WEB-DL.EAC9.2-GROUP.mkv", "Baz Qux", "2024"},
		{"Standalone.EAC.2024.1080p.WEB-DL.EAC3-GROUP.mkv", "Standalone EAC", "2024"},
		// Generalized codec+channel: AAC/DDP/MA/AC3 with shorthand or 3-digit forms.
		{"Movie.A.2024.1080p.WEB-DL.AAC2.0-GROUP.mkv", "Movie A", "2024"},
		{"Movie.B.2024.1080p.WEB-DL.DDP5.1-GROUP.mkv", "Movie B", "2024"},
		{"Movie.C.2024.1080p.WEB-DL.AC3.5.1-GROUP.mkv", "Movie C", "2024"},
		{"Movie.D.2024.1080p.WEB-DL.MA7.1-GROUP.mkv", "Movie D", "2024"},
		// x264/x265 unified pattern still strips both.
		{"Movie.E.2024.1080p.WEB-DL.x264-GROUP.mkv", "Movie E", "2024"},
		{"Movie.F.2024.1080p.WEB-DL.x265-GROUP.mkv", "Movie F", "2024"},
		// OPUS audio with channel suffix shorthand (YorMama-style).
		{"2.Guns.2013.1080p.BluRay.OPUS5.1.AV1-YorMama.mkv", "2 Guns", "2013"},
		// 10bits plural (Rapta-style); previously only "10bit" was stripped.
		{"Greenland.2.Migration.2026.1080p.WEBRip.AAC5.1.10bits.x265-Rapta.mkv", "Greenland 2 Migration", "2026"},
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
