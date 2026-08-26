package config

// TVPaths returns all ordinary and seeding TV intake roots.
func (w WatchConfig) TVPaths() []string {
	paths := make([]string, 0, len(w.TV)+len(w.SeedingTV))
	paths = append(paths, w.TV...)
	return append(paths, w.SeedingTV...)
}

// MoviePaths returns all ordinary and seeding movie intake roots.
func (w WatchConfig) MoviePaths() []string {
	paths := make([]string, 0, len(w.Movies)+len(w.SeedingMovies))
	paths = append(paths, w.Movies...)
	return append(paths, w.SeedingMovies...)
}

// AllPaths returns every configured intake root in daemon scan order.
func (w WatchConfig) AllPaths() []string {
	paths := w.MoviePaths()
	return append(paths, w.TVPaths()...)
}
