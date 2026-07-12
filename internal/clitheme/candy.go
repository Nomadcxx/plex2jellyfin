package clitheme

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// CandyBar renders an ILoveCandy-style pacman progress bar (pct in [0,1]).
// Eaten trail = '-', pacman = 'C', remaining pellets = 'o'.
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
	var b strings.Builder
	b.Grow(width + 2)
	b.WriteByte('[')
	if pct <= 0 {
		b.WriteString(strings.Repeat("o", width))
		b.WriteByte(']')
		return b.String()
	}
	if pct >= 1 {
		b.WriteString(strings.Repeat("-", width-1))
		b.WriteByte('C')
		b.WriteByte(']')
		return b.String()
	}
	pos := int(pct * float64(width-1))
	for i := 0; i < width; i++ {
		switch {
		case i < pos:
			b.WriteByte('-')
		case i == pos:
			b.WriteByte('C')
		default:
			b.WriteByte('o')
		}
	}
	b.WriteByte(']')
	return b.String()
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

	lines := 0
	for i, lib := range u.Libs {
		label := filepath.Base(strings.TrimSuffix(lib, "/"))
		if label == "" {
			label = lib
		}
		label = truncateRunes(label, 26)

		var bar, status string
		switch {
		case u.done[i] || active >= len(u.Libs):
			bar = CandyBar(1, u.Width)
			status = fmt.Sprintf("%d files", u.files[i])
		case i == active:
			bar = CandyBar(SoftCandyPct(u.files[i]), u.Width)
			status = "scanning"
		default:
			bar = CandyBar(0, u.Width)
			status = "queued"
		}
		fmt.Fprintf(u.Out, "  %s %s  %s\033[K\n", padRight(label, 26), bar, status)
		lines++
	}
	fmt.Fprintf(u.Out, "  total files indexed: %d\033[K\n", filesScanned)
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
