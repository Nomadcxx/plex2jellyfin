// cmd/installer/animations.go
package main

import (
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// BeamsEffect implements beams that travel across rows and columns
type BeamsEffect struct {
	width   int
	height  int
	palette []string
	frame   int

	rowSymbols    []rune
	columnSymbols []rune
	delay         int
	speedRange    [2]int

	activeBeams   []Beam
	pendingBeams  []Beam
	nextBeamDelay int
}

// Beam represents a single beam traveling across the screen
type Beam struct {
	X         int
	Y         int
	Direction string // "row" or "column"
	Length    int
	Speed     float64
	Position  float64
	Symbol    rune
	Color     string
}

// NewBeamsEffect creates a new beams effect
func NewBeamsEffect(width, height int) *BeamsEffect {
	// Monochrome palette
	palette := []string{"#ffffff", "#cccccc", "#999999", "#666666"}

	b := &BeamsEffect{
		width:         width,
		height:        height,
		palette:       palette,
		frame:         0,
		rowSymbols:    []rune{'▂', '▁', '_'},
		columnSymbols: []rune{'▌', '▍', '▎', '▏'},
		delay:         10,
		speedRange:    [2]int{10, 40},
		activeBeams:   []Beam{},
		pendingBeams:  []Beam{},
		nextBeamDelay: 0,
	}
	b.init()
	return b
}

func (b *BeamsEffect) init() {
	b.pendingBeams = make([]Beam, 0, (b.height+b.width)*2)

	// Add row beams
	for y := 0; y < b.height; y++ {
		beam := Beam{
			X:         0,
			Y:         y,
			Direction: "row",
			Length:    3 + rand.Intn(5),
			Speed:     float64(rand.Intn(b.speedRange[1]-b.speedRange[0])+b.speedRange[0]) * 0.1,
			Position:  -float64(b.width),
			Symbol:    b.rowSymbols[rand.Intn(len(b.rowSymbols))],
			Color:     b.palette[rand.Intn(len(b.palette))],
		}
		b.pendingBeams = append(b.pendingBeams, beam)
	}

	// Add column beams
	for x := 0; x < b.width; x++ {
		beam := Beam{
			X:         x,
			Y:         0,
			Direction: "column",
			Length:    3 + rand.Intn(5),
			Speed:     float64(rand.Intn(b.speedRange[1]-b.speedRange[0])+b.speedRange[0]) * 0.1,
			Position:  -float64(b.height),
			Symbol:    b.columnSymbols[rand.Intn(len(b.columnSymbols))],
			Color:     b.palette[rand.Intn(len(b.palette))],
		}
		b.pendingBeams = append(b.pendingBeams, beam)
	}

	rand.Shuffle(len(b.pendingBeams), func(i, j int) {
		b.pendingBeams[i], b.pendingBeams[j] = b.pendingBeams[j], b.pendingBeams[i]
	})
}

func (b *BeamsEffect) Resize(width, height int) {
	b.width = width
	b.height = height
	b.init()
}

func (b *BeamsEffect) Update() {
	b.frame++

	if b.nextBeamDelay > 0 {
		b.nextBeamDelay--
	}

	if b.nextBeamDelay == 0 && len(b.pendingBeams) > 0 {
		groupSize := rand.Intn(5) + 1
		for i := 0; i < groupSize && len(b.pendingBeams) > 0; i++ {
			beam := b.pendingBeams[0]
			b.activeBeams = append(b.activeBeams, beam)
			b.pendingBeams = b.pendingBeams[1:]
		}
		b.nextBeamDelay = b.delay
	}

	for i := range b.activeBeams {
		b.activeBeams[i].Position += b.activeBeams[i].Speed
	}

	filtered := b.activeBeams[:0]
	for _, beam := range b.activeBeams {
		if beam.Direction == "row" {
			if beam.Position > -float64(beam.Length) && beam.Position < float64(b.width) {
				filtered = append(filtered, beam)
			}
		} else {
			if beam.Position > -float64(beam.Length) && beam.Position < float64(b.height) {
				filtered = append(filtered, beam)
			}
		}
	}
	b.activeBeams = filtered

	if len(b.activeBeams) == 0 && len(b.pendingBeams) == 0 {
		b.init()
	}
}

func (b *BeamsEffect) Render() string {
	canvas := make([][]rune, b.height)
	colors := make([][]string, b.height)
	for i := range canvas {
		canvas[i] = make([]rune, b.width)
		colors[i] = make([]string, b.width)
		for j := range canvas[i] {
			canvas[i][j] = ' '
			colors[i][j] = ""
		}
	}

	for _, beam := range b.activeBeams {
		if beam.Direction == "row" {
			startX := int(beam.Position)
			for i := 0; i < beam.Length; i++ {
				x := startX + i
				if x >= 0 && x < b.width && beam.Y >= 0 && beam.Y < b.height {
					canvas[beam.Y][x] = beam.Symbol
					colors[beam.Y][x] = beam.Color
				}
			}
		} else {
			startY := int(beam.Position)
			for i := 0; i < beam.Length; i++ {
				y := startY + i
				if y >= 0 && y < b.height && beam.X >= 0 && beam.X < b.width {
					canvas[y][beam.X] = beam.Symbol
					colors[y][beam.X] = beam.Color
				}
			}
		}
	}

	var lines []string
	for y := 0; y < b.height; y++ {
		var line strings.Builder
		for x := 0; x < b.width; x++ {
			char := canvas[y][x]
			if char != ' ' && colors[y][x] != "" {
				styled := lipgloss.NewStyle().
					Foreground(lipgloss.Color(colors[y][x])).
					Render(string(char))
				line.WriteString(styled)
			} else {
				line.WriteRune(char)
			}
		}
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

// TypewriterTicker types out roasts one character at a time
type TypewriterTicker struct {
	roasts       []string
	roastsRunes  [][]rune // Pre-converted to runes for proper Unicode handling
	roastIndex   int
	charIndex    int // Index into runes, not bytes
	lastUpdate   time.Time
	charDelay    time.Duration
	messageDelay time.Duration
	paused       bool
	pauseUntil   time.Time
}

func NewTypewriterTicker() *TypewriterTicker {
	roasts := make([]string, len(jellyWatchRoasts))
	copy(roasts, jellyWatchRoasts)
	rand.Shuffle(len(roasts), func(i, j int) {
		roasts[i], roasts[j] = roasts[j], roasts[i]
	})

	// Pre-convert all roasts to runes for proper Unicode handling
	roastsRunes := make([][]rune, len(roasts))
	for i, r := range roasts {
		roastsRunes[i] = []rune(r)
	}

	return &TypewriterTicker{
		roasts:       roasts,
		roastsRunes:  roastsRunes,
		roastIndex:   0,
		charIndex:    0,
		lastUpdate:   time.Now(),
		charDelay:    time.Millisecond * 50,
		messageDelay: time.Second * 2,
		paused:       false,
		pauseUntil:   time.Now(),
	}
}

func (t *TypewriterTicker) Update() {
	now := time.Now()

	if t.paused {
		if now.Before(t.pauseUntil) {
			return
		}
		t.roastIndex = (t.roastIndex + 1) % len(t.roasts)
		t.charIndex = 0
		t.paused = false
		t.lastUpdate = now
		return
	}

	if now.Sub(t.lastUpdate) >= t.charDelay {
		// Use rune length for proper Unicode character counting
		currentRunes := t.roastsRunes[t.roastIndex]
		if t.charIndex >= len(currentRunes) {
			t.paused = true
			t.pauseUntil = now.Add(t.messageDelay)
		} else {
			t.charIndex++
		}
		t.lastUpdate = now
	}
}

func (t *TypewriterTicker) Render(width int) string {
	if len(t.roasts) == 0 {
		return strings.Repeat(" ", width)
	}

	currentRunes := t.roastsRunes[t.roastIndex]
	var result string

	if t.paused {
		result = t.roasts[t.roastIndex]
	} else {
		// Use rune slicing to properly handle Unicode characters
		result = string(currentRunes[:t.charIndex]) + "█"
	}

	// Calculate display width using rune count for proper Unicode handling
	resultRunes := []rune(result)
	resultLen := len(resultRunes)

	// Center it
	if resultLen < width {
		padding := (width - resultLen) / 2
		result = strings.Repeat(" ", padding) + result + strings.Repeat(" ", width-resultLen-padding)
	} else if resultLen > width {
		result = string(resultRunes[:width])
	}

	return result
}
