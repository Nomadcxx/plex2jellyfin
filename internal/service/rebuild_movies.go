package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

type MovieRebuildResult struct {
	FilesConsidered int
	MoviesRebuilt   int
}

type movieFileGroup struct {
	title string
	year  int
	files []*database.MediaFile
}

func (s *CleanupService) RebuildMoviesFromMediaFiles(movieLibraries []string) (*MovieRebuildResult, error) {
	files, err := s.db.GetAllMediaFiles()
	if err != nil {
		return nil, fmt.Errorf("loading media files: %w", err)
	}

	result := &MovieRebuildResult{}
	groups := make(map[string]*movieFileGroup)

	for _, file := range files {
		if file == nil || file.MediaType != "movie" || file.Year == nil {
			continue
		}
		if !mediaFileInLibraries(file, movieLibraries) {
			continue
		}
		result.FilesConsidered++
		key := fmt.Sprintf("%s|%d", file.NormalizedTitle, *file.Year)
		group := groups[key]
		if group == nil {
			group = &movieFileGroup{
				title: titleForMovieFile(file),
				year:  *file.Year,
			}
			groups[key] = group
		}
		group.files = append(group.files, file)
	}

	for _, group := range groups {
		if len(group.files) == 0 {
			continue
		}
		sort.SliceStable(group.files, func(i, j int) bool {
			a, b := group.files[i], group.files[j]
			if a.QualityScore != b.QualityScore {
				return a.QualityScore > b.QualityScore
			}
			if a.Size != b.Size {
				return a.Size > b.Size
			}
			return a.ID < b.ID
		})

		best := group.files[0]
		movie := &database.Movie{
			Title:          group.title,
			Year:           group.year,
			CanonicalPath:  filepath.Dir(best.Path),
			LibraryRoot:    best.LibraryRoot,
			Source:         "filesystem",
			SourcePriority: 50,
		}
		if _, err := s.db.UpsertMovie(movie); err != nil {
			return nil, fmt.Errorf("upserting rebuilt movie %s (%d): %w", group.title, group.year, err)
		}
		for _, file := range group.files {
			file.ParentMovieID = &movie.ID
			if err := s.db.UpdateMediaFile(file); err != nil {
				return nil, fmt.Errorf("linking media file %d to movie %d: %w", file.ID, movie.ID, err)
			}
		}
		bestID := best.ID
		if err := s.db.UpdateMovieBestFile(movie.ID, &bestID); err != nil {
			return nil, fmt.Errorf("updating best file for movie %d: %w", movie.ID, err)
		}
		result.MoviesRebuilt++
	}

	return result, nil
}

func mediaFileInLibraries(file *database.MediaFile, libraries []string) bool {
	for _, root := range libraries {
		if file.LibraryRoot == root {
			return true
		}
		rel, err := filepath.Rel(root, file.Path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func titleForMovieFile(file *database.MediaFile) string {
	if info, err := naming.ParseMovieName(filepath.Base(filepath.Dir(file.Path))); err == nil && info.Title != "" {
		return info.Title
	}
	if info, err := naming.ParseMovieName(filepath.Base(file.Path)); err == nil && info.Title != "" {
		return info.Title
	}
	return file.NormalizedTitle
}
