package identity

import "testing"

func TestCompareSeries_DifferentYearsBlock(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Utopia", Year: 2014, Path: "/tv/Utopia (AU) (2014)"},
		SeriesIdentity{Title: "Utopia", Year: 2013, Path: "/tv/Utopia (2013)"},
	)
	if got.Verdict != VerdictDifferent {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictDifferent, got.Reasons)
	}
}

func TestCompareSeries_RegionConflictBlocks(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Survivor AU", Year: 2016, Path: "/tv/Survivor AU (2016)"},
		SeriesIdentity{Title: "Survivor", Year: 2000, Path: "/tv/Survivor (2000)"},
	)
	if got.Verdict != VerdictDifferent {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictDifferent, got.Reasons)
	}
}

func TestCompareSeries_SameTitleAndYearAllowed(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Silo", Year: 2023, Path: "/tv1/Silo (2023)"},
		SeriesIdentity{Title: "Silo", Year: 2023, Path: "/tv2/Silo (2023)"},
	)
	if got.Verdict != VerdictSame {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictSame, got.Reasons)
	}
}

func TestCompareSeries_RegionSuffixIsManualReview(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Ghosts", Year: 2021, Path: "/tv/Ghosts (2021)"},
		SeriesIdentity{Title: "Ghosts US", Year: 2021, Path: "/tv/Ghosts US (2021)"},
	)
	if got.Verdict != VerdictUncertain {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictUncertain, got.Reasons)
	}
}

func TestCompareSeries_ApostropheNormalized(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Attenborough's Life in Colour", Year: 2021, Path: "/tv/Attenborough's Life in Colour (2021)"},
		SeriesIdentity{Title: "attenboroughs life in colour", Year: 0, Path: "/tv/attenboroughs.life.in.colour.s01e02.1080p.bluray.x264-orbs.mkv"},
	)
	if got.Verdict != VerdictSame {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictSame, got.Reasons)
	}
}

func TestCompareSeries_ApostropheNormalizedHistory(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "History's Greatest Mysteries", Year: 2020, Path: "/tv/History's Greatest Mysteries (2020)"},
		SeriesIdentity{Title: "historys greatest mysteries", Year: 0, Path: "/tv/historys.greatest.mysteries.s02e01.1080p.web.h264-whosnext.mkv"},
	)
	if got.Verdict != VerdictSame {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictSame, got.Reasons)
	}
}

func TestCompareSeries_EpisodeYearNotSeriesYear(t *testing.T) {
	got := CompareSeries(
		SeriesIdentity{Title: "Grand Designs", Year: 1999, Path: "/tv/Grand Designs (1999)"},
		SeriesIdentity{Title: "Grand Designs", Year: 0, Path: "/tv/Grand.Designs.S24E08.Billingshurst.Revisit.2023.1080p.ALL4.WEB-DL.AAC2.0.H.264.mkv"},
	)
	if got.Verdict != VerdictSame {
		t.Fatalf("Verdict = %s, want %s: %#v", got.Verdict, VerdictSame, got.Reasons)
	}
}
