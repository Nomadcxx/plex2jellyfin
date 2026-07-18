package naming

import "testing"

func TestIsExtrasRelease(t *testing.T) {
	yes := []string{
		"The.Last.Ship.S02.BONUS.2015.BluRay.1080p.AC3.x264-MTeam-Obfuscated",
		"Show.S01.EXTRAS.1080p.WEB",
		"Show.S03.Behind.The.Scenes.720p",
		"Show.Featurettes.BluRay",
		"Show S02 Extra Features",
	}
	no := []string{
		"The.Last.Ship.S02.2015.BluRay.1080p",
		"Bonus.Family.S01E01.720p", // real show titled "Bonus Family" with an episode marker
		"The.Extras.S01E03.1080p.WEB", // real show titled "The Extras"
		"Plain.Show.S04.1080p.BluRay",
	}
	for _, name := range yes {
		if !IsExtrasRelease(name) {
			t.Errorf("IsExtrasRelease(%q) = false, want true", name)
		}
	}
	for _, name := range no {
		if IsExtrasRelease(name) {
			t.Errorf("IsExtrasRelease(%q) = true, want false", name)
		}
	}
}

func TestCleanExtrasName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"The.Last.Ship.S02.BONUS.2015.BluRay.1080p.AC3.x264-MTeam-Obfuscated", "The Last Ship S02 BONUS"},
		{"Show.S01.EXTRAS", "Show S01 EXTRAS"},
		{"Behind.The.Scenes", "Behind The Scenes"},
	}
	for _, c := range cases {
		if got := CleanExtrasName(c.in); got != c.want {
			t.Errorf("CleanExtrasName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
