package clitheme

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// CandyBar renders an ILoveCandy-style pacman progress bar (pct in [0,1]).
// Eaten trail = '-', pacman = 'C', remaining pellets = 'o'.
// Uses the same Plex-yellow lipgloss palette as FangScheme (Fang itself only
// styles Cobra help/errors; progress chrome shares the brand colors).
func CandyBar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	// Whole eaten run (----C) is Plex yellow; uneaten pellets stay dim.
	trail := lipgloss.NewStyle().Foreground(PlexYellow)
	pacman := lipgloss.NewStyle().Bold(true).Foreground(PlexYellow)
	pellet := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	bracket := lipgloss.NewStyle().Foreground(PlexYellow)

	var inner strings.Builder
	inner.Grow(width * 8)
	if pct <= 0 {
		inner.WriteString(pellet.Render(strings.Repeat("o", width)))
	} else if pct >= 1 {
		inner.WriteString(trail.Render(strings.Repeat("-", width-1)))
		inner.WriteString(pacman.Render("C"))
	} else {
		pos := int(pct * float64(width-1))
		for i := 0; i < width; i++ {
			switch {
			case i < pos:
				inner.WriteString(trail.Render("-"))
			case i == pos:
				inner.WriteString(pacman.Render("C"))
			default:
				inner.WriteString(pellet.Render("o"))
			}
		}
	}
	return bracket.Render("[") + inner.String() + bracket.Render("]")
}

// SoftCandyPct maps an unbounded counter (files seen in the active library)
// onto (0,1) so the pacman keeps moving without a pre-count.
func SoftCandyPct(n int) float64 {
	if n <= 0 {
		return 0.02
	}
	// Asymptotic toward ~0.92 until the library is marked done.
	return 1 - 1/((float64(n)/40)+1.05)
}

// LibraryLabel turns /mnt/STORAGE1/TVSHOWS into STORAGE1/TVSHOWS so sibling
// mounts with the same leaf name stay distinguishable in the candy UI.
func LibraryLabel(path string, max int) string {
	path = strings.TrimRight(filepath.Clean(path), "/")
	if path == "" || path == "." {
		return truncateRunes("?", max)
	}
	base := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	label := base
	if parent != "" && parent != "." && parent != "/" && parent != string(filepath.Separator) {
		label = parent + "/" + base
	}
	return truncateRunes(label, max)
}

// LibraryCandyUI draws one ILoveCandy bar per library and redraws in place.
type LibraryCandyUI struct {
	Out   io.Writer
	Libs  []string
	Width int // bar inner width; default 22

	mu       sync.Mutex
	done     []bool
	files    []int
	baseline int // FilesScanned when current library started
	drawn    bool
	lines    int
}

// Render updates library i as active (or completes libs below LibrariesDone)
// using absolute filesScanned from the scanner.
func (u *LibraryCandyUI) Render(librariesDone, filesScanned int, currentLib string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Width < 4 {
		u.Width = 22
	}
	n := len(u.Libs)
	if len(u.done) != n {
		u.done = make([]bool, n)
		u.files = make([]int, n)
	}

	active := librariesDone
	if active < 0 {
		active = 0
	}
	if active > n {
		active = n
	}

	// Mark completed libraries.
	for i := 0; i < active && i < n; i++ {
		if !u.done[i] {
			u.done[i] = true
			if u.files[i] == 0 {
				u.files[i] = filesScanned - u.baseline
				if u.files[i] < 0 {
					u.files[i] = 0
				}
			}
			u.baseline = filesScanned
		}
	}
	if active < n && currentLib != "" {
		// Keep files tally for the active row as delta since library start.
		u.files[active] = filesScanned - u.baseline
		if u.files[active] < 0 {
			u.files[active] = 0
		}
	}
	if active >= n {
		for i := range u.done {
			u.done[i] = true
		}
	}

	u.redraw(active, filesScanned)
}

func (u *LibraryCandyUI) redraw(active, filesScanned int) {
	if u.drawn && u.lines > 0 {
		// Move cursor up to rewrite the block.
		fmt.Fprintf(u.Out, "\033[%dA", u.lines)
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#d4d4d4"))
	activeLabel := lipgloss.NewStyle().Bold(true).Foreground(PlexYellow)
	doneLabel := lipgloss.NewStyle().Foreground(successColor)
	queuedStatus := mutedStyle()
	scanStatus := lipgloss.NewStyle().Foreground(PlexYellow)
	doneStatus := successStyle()
	totalStyle := mutedStyle()

	const labelWidth = 28
	lines := 0
	for i, lib := range u.Libs {
		label := LibraryLabel(lib, labelWidth)
		label = padRight(label, labelWidth)

		var bar, status, row string
		switch {
		case u.done[i] || active >= len(u.Libs):
			bar = CandyBar(1, u.Width)
			status = doneStatus.Render(fmt.Sprintf("%d files", u.files[i]))
			row = doneLabel.Render(label) + " " + bar + "  " + status
		case i == active:
			bar = CandyBar(SoftCandyPct(u.files[i]), u.Width)
			status = scanStatus.Render("scanning")
			row = activeLabel.Render(label) + " " + bar + "  " + status
		default:
			bar = CandyBar(0, u.Width)
			status = queuedStatus.Render("queued")
			row = labelStyle.Render(label) + " " + bar + "  " + status
		}
		fmt.Fprintf(u.Out, "  %s\033[K\n", row)
		lines++
	}
	fmt.Fprintf(u.Out, "  %s\033[K\n", totalStyle.Render(fmt.Sprintf("total files indexed: %d", filesScanned)))
	lines++

	u.lines = lines
	u.drawn = true
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func truncateRunes(s string, max int) string {
	if max < 2 {
		max = 2
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	i := 0
	for _, r := range s {
		if i >= max-1 {
			break
		}
		b.WriteRune(r)
		i++
	}
	b.WriteRune('…')
	return b.String()
}
