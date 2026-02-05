package naming

import "strings"

// knownMediaTitles contains lowercase titles that are legitimate media
// but also appear in the release group blacklist.
// This list is curated from common false positives.
var knownMediaTitles = map[string]bool{
	// TV Shows that conflict with release group names
	"barry":            true, // HBO series
	"westworld":        true, // HBO series
	"ted":              true, // Peacock series
	"lasso":            true, // part of "Ted Lasso"
	"tedlasso":         true, // normalized form
	"ragnarok":         true, // Netflix series
	"rome":             true, // HBO series
	"fargo":            true, // FX series
	"dexter":           true, // Showtime series
	"lost":             true, // ABC series
	"fringe":           true, // Fox series
	"castle":           true, // ABC series
	"chuck":            true, // NBC series
	"flash":            true, // CW series
	"arrow":            true, // CW series
	"bones":            true, // Fox series
	"house":            true, // Fox series
	"monk":             true, // USA series
	"suits":            true, // USA series
	"narcos":           true, // Netflix series
	"ozark":            true, // Netflix series
	"mindhunter":       true, // Netflix series
	"lucifer":          true, // Netflix series
	"euphoria":         true, // HBO series
	"succession":       true, // HBO series
	"silicon":          true, // part of "Silicon Valley"
	"siliconvalley":    true, // normalized form
	"veep":             true, // HBO series
	"ballers":          true, // HBO series
	"entourage":        true, // HBO series
	"girls":            true, // HBO series
	"true":             true, // part of "True Detective", "True Blood"
	"truedetective":    true, // normalized
	"trueblood":        true, // normalized
	"lovecraft":        true, // part of "Lovecraft Country"
	"lovecraftcountry": true, // normalized
	"harley":           true, // part of "Harley Quinn"
	"harleyquinn":      true, // normalized
	"avenue":           true, // part of "Avenue 5"
	"avenue5":          true, // normalized
	"project":          true, // part of various shows
	"projectrunway":    true, // normalized
	"babylon":          true, // part of "Babylon 5"
	"babylon5":         true, // normalized

	// Movies that conflict
	"her":       true, // 2013 film
	"it":        true, // Stephen King films
	"us":        true, // Jordan Peele film
	"up":        true, // Pixar film
	"jaws":      true, // Spielberg classic
	"heat":      true, // Michael Mann film
	"drive":     true, // 2011 film
	"gravity":   true, // 2013 film
	"prisoners": true, // 2013 film
	"arrival":   true, // 2016 film
	"sicario":   true, // 2015 film
	"zodiac":    true, // 2007 film
	"seven":     true, // 1995 film (Se7en)
	"se7en":     true, // alternate
	"saw":       true, // horror franchise
	"scream":    true, // horror franchise
	"halloween": true, // horror franchise
	"predator":  true, // action franchise
	"alien":     true, // sci-fi franchise
	"aliens":    true, // sci-fi franchise
}

// IsKnownMediaTitle checks if a title is a known legitimate media title
// that happens to conflict with release group names.
func IsKnownMediaTitle(title string) bool {
	if title == "" {
		return false
	}
	return knownMediaTitles[strings.ToLower(title)]
}
