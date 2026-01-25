package naming

// SourceHint indicates where a file originated from
type SourceHint int

const (
	SourceUnknown SourceHint = iota // Unknown origin, use filename-only detection
	SourceTV                        // File came from TV watch folder
	SourceMovie                     // File came from Movie watch folder
)

// String returns a human-readable representation of the hint
func (h SourceHint) String() string {
	switch h {
	case SourceTV:
		return "tv"
	case SourceMovie:
		return "movie"
	default:
		return "unknown"
	}
}
