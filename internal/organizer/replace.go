package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

// trackUntrackedReplacement inserts a source row so an upgrade can use the
// reserved quarantine in MoveMediaFileWithReplacement. Destination metadata is
// copied onto that row so a tracked library file keeps its identity and links.
func (o *Organizer) trackUntrackedReplacement(sourcePath, targetPath, libraryRoot, mediaType string) (*database.MediaFile, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("stat replacement source: %w", err)
	}
	dest, err := o.db.GetMediaFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read replacement destination: %w", err)
	}
	file := &database.MediaFile{
		Path:            sourcePath,
		Size:            info.Size(),
		ModifiedAt:      info.ModTime(),
		MediaType:       mediaType,
		NormalizedTitle: database.NormalizeForMatch(strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))),
		LibraryRoot:     libraryRoot,
		Source:          "plex2jellyfin",
		SourcePriority:  100,
	}
	if dest != nil {
		file.MediaType = dest.MediaType
		file.ParentMovieID = dest.ParentMovieID
		file.ParentSeriesID = dest.ParentSeriesID
		file.ParentEpisodeID = dest.ParentEpisodeID
		file.NormalizedTitle = dest.NormalizedTitle
		file.Year = dest.Year
		file.Season = dest.Season
		file.Episode = dest.Episode
		file.Resolution = dest.Resolution
		file.SourceType = dest.SourceType
		file.Codec = dest.Codec
		file.AudioFormat = dest.AudioFormat
		file.QualityScore = dest.QualityScore
		file.Confidence = dest.Confidence
		file.NeedsReview = dest.NeedsReview
		file.ParseMethod = dest.ParseMethod
		file.IsJellyfinCompliant = dest.IsJellyfinCompliant
		file.ComplianceIssues = dest.ComplianceIssues
		if dest.Source != "" {
			file.Source = dest.Source
		}
		if dest.SourcePriority != 0 {
			file.SourcePriority = dest.SourcePriority
		}
		if file.LibraryRoot == "" {
			file.LibraryRoot = dest.LibraryRoot
		}
	}
	if file.MediaType != "movie" && file.MediaType != "episode" {
		return nil, nil
	}
	if file.NormalizedTitle == "" {
		return nil, nil
	}
	if err := o.db.UpsertMediaFile(file); err != nil {
		return nil, fmt.Errorf("track replacement source: %w", err)
	}
	return file, nil
}

// replaceUntrackedDestination quarantines an existing destination, publishes
// the source into that path, and restores the old file if the transfer fails.
func (o *Organizer) replaceUntrackedDestination(sourcePath, targetPath string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	targetInfo, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		result, moveErr := o.transferer.Move(sourcePath, targetPath, opts)
		if moveErr == nil {
			moveErr = result.Validate()
		}
		if result == nil {
			result = &transfer.TransferResult{Error: moveErr}
		}
		return result, moveErr
	}
	if err != nil {
		return nil, fmt.Errorf("inspect replacement destination %s: %w", targetPath, err)
	}
	contentInfo, err := os.Stat(targetPath)
	if err != nil || !contentInfo.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("destination is not a regular file")
		}
		return nil, fmt.Errorf("inspect replacement destination %s: %w", targetPath, err)
	}
	identity, ok := fileidentity.FromInfo(targetInfo)
	if !ok {
		return nil, fmt.Errorf("replacement destination has no stable file identity: %s", targetPath)
	}
	quarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		return nil, err
	}
	if err := fileidentity.MoveToQuarantine(targetPath, quarantinePath, identity, true); err != nil {
		return nil, fmt.Errorf("quarantine replacement destination: %w", err)
	}
	result, err := o.transferer.Move(sourcePath, targetPath, opts)
	if err == nil {
		err = result.Validate()
	}
	if err != nil {
		if restoreErr := fileidentity.RestoreQuarantine(quarantinePath, targetPath, identity, true); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore replacement destination: %w", restoreErr))
		}
		if result == nil {
			result = &transfer.TransferResult{}
		}
		result.Error = err
		return result, err
	}
	if removeErr := fileidentity.RemoveQuarantine(quarantinePath, identity, true); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return result, fmt.Errorf("remove replaced destination backup: %w", removeErr)
	}
	return result, nil
}
