package migration

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

type PathMismatch struct {
	MediaType    string
	ID           int64
	Title        string
	Year         int
	DatabasePath string
	SonarrPath   string
	RadarrPath   string
	SonarrID     *int
	RadarrID     *int
	HasSonarrID  bool
	HasRadarrID  bool
}

func DetectSeriesMismatches(db *database.MediaDB, sonarrClient *sonarr.Client) ([]PathMismatch, error) {
	if sonarrClient == nil {
		return nil, fmt.Errorf("getting series from Sonarr: nil client")
	}

	allSeries, err := db.GetAllSeries()
	if err != nil {
		return nil, fmt.Errorf("getting all series from DB: %w", err)
	}

	sonarrSeries, err := sonarrClient.GetAllSeries()
	if err != nil {
		return nil, fmt.Errorf("getting series from Sonarr: %w", err)
	}

	sonarrMap := make(map[int]sonarr.Series)
	for _, s := range sonarrSeries {
		sonarrMap[s.ID] = s
	}

	var mismatches []PathMismatch
	for _, dbSeries := range allSeries {
		if dbSeries.SonarrID == nil {
			continue
		}

		sonarrSeries, exists := sonarrMap[*dbSeries.SonarrID]
		if !exists {
			continue
		}

		if dbSeries.CanonicalPath != sonarrSeries.Path {
			mismatches = append(mismatches, PathMismatch{
				MediaType:    "series",
				ID:           dbSeries.ID,
				Title:        dbSeries.Title,
				Year:         dbSeries.Year,
				DatabasePath: dbSeries.CanonicalPath,
				SonarrPath:   sonarrSeries.Path,
				SonarrID:     dbSeries.SonarrID,
				HasSonarrID:  true,
			})
		}
	}

	return mismatches, nil
}

func DetectMovieMismatches(db *database.MediaDB, radarrClient *radarr.Client) ([]PathMismatch, error) {
	if radarrClient == nil {
		return nil, fmt.Errorf("getting movies from Radarr: nil client")
	}

	allMovies, err := db.GetAllMovies()
	if err != nil {
		return nil, fmt.Errorf("getting all movies from DB: %w", err)
	}

	radarrMovies, err := radarrClient.GetMovies()
	if err != nil {
		return nil, fmt.Errorf("getting movies from Radarr: %w", err)
	}

	radarrMap := make(map[int]radarr.Movie)
	for _, m := range radarrMovies {
		radarrMap[m.ID] = m
	}

	var mismatches []PathMismatch
	for _, dbMovie := range allMovies {
		if dbMovie.RadarrID == nil {
			continue
		}

		radarrMovie, exists := radarrMap[*dbMovie.RadarrID]
		if !exists {
			continue
		}

		if dbMovie.CanonicalPath != radarrMovie.Path {
			mismatches = append(mismatches, PathMismatch{
				MediaType:    "movie",
				ID:           dbMovie.ID,
				Title:        dbMovie.Title,
				Year:         dbMovie.Year,
				DatabasePath: dbMovie.CanonicalPath,
				RadarrPath:   radarrMovie.Path,
				RadarrID:     dbMovie.RadarrID,
				HasRadarrID:  true,
			})
		}
	}

	return mismatches, nil
}

type FixChoice string

const (
	FixChoiceKeepJellyWatch   FixChoice = "jellywatch"
	FixChoiceKeepSonarrRadarr FixChoice = "arr"
	FixChoiceSkip             FixChoice = "skip"
)

func FixSeriesMismatch(db *database.MediaDB, sonarrClient *sonarr.Client, mismatch PathMismatch, choice FixChoice) error {
	if mismatch.MediaType != "series" {
		return fmt.Errorf("not a series mismatch")
	}

	switch choice {
	case FixChoiceKeepJellyWatch:
		if mismatch.SonarrID == nil {
			return fmt.Errorf("series has no Sonarr ID")
		}
		if err := sonarrClient.UpdateSeriesPath(*mismatch.SonarrID, mismatch.DatabasePath); err != nil {
			return fmt.Errorf("updating Sonarr path: %w", err)
		}
		if err := db.MarkSeriesSynced(mismatch.ID); err != nil {
			return fmt.Errorf("marking series synced: %w", err)
		}

	case FixChoiceKeepSonarrRadarr:
		series, err := db.GetSeriesByID(mismatch.ID)
		if err != nil {
			return fmt.Errorf("getting series: %w", err)
		}
		if series == nil {
			return fmt.Errorf("series not found: %d", mismatch.ID)
		}

		series.CanonicalPath = mismatch.SonarrPath
		series.Source = "sonarr"
		series.SourcePriority = 50

		_, err = db.UpsertSeries(series)
		if err != nil {
			return fmt.Errorf("updating series in DB: %w", err)
		}
		if err := db.MarkSeriesSynced(mismatch.ID); err != nil {
			return fmt.Errorf("marking series synced: %w", err)
		}

	case FixChoiceSkip:
		return nil

	default:
		return fmt.Errorf("invalid choice: %s", choice)
	}

	return nil
}

func FixMovieMismatch(db *database.MediaDB, radarrClient *radarr.Client, mismatch PathMismatch, choice FixChoice) error {
	if mismatch.MediaType != "movie" {
		return fmt.Errorf("not a movie mismatch")
	}

	switch choice {
	case FixChoiceKeepJellyWatch:
		if mismatch.RadarrID == nil {
			return fmt.Errorf("movie has no Radarr ID")
		}
		if err := radarrClient.UpdateMoviePath(*mismatch.RadarrID, mismatch.DatabasePath); err != nil {
			return fmt.Errorf("updating Radarr path: %w", err)
		}
		if err := db.MarkMovieSynced(mismatch.ID); err != nil {
			return fmt.Errorf("marking movie synced: %w", err)
		}

	case FixChoiceKeepSonarrRadarr:
		movie, err := db.GetMovieByID(mismatch.ID)
		if err != nil {
			return fmt.Errorf("getting movie: %w", err)
		}
		if movie == nil {
			return fmt.Errorf("movie not found: %d", mismatch.ID)
		}

		movie.CanonicalPath = mismatch.RadarrPath
		movie.Source = "radarr"
		movie.SourcePriority = 50

		_, err = db.UpsertMovie(movie)
		if err != nil {
			return fmt.Errorf("updating movie in DB: %w", err)
		}
		if err := db.MarkMovieSynced(mismatch.ID); err != nil {
			return fmt.Errorf("marking movie synced: %w", err)
		}

	case FixChoiceSkip:
		return nil

	default:
		return fmt.Errorf("invalid choice: %s", choice)
	}

	return nil
}
