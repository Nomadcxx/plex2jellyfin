package service

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

// CleanupService provides operations for cleaning up media libraries
type CleanupService struct {
	db *database.MediaDB
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(db *database.MediaDB) *CleanupService {
	return &CleanupService{db: db}
}

// DeleteFileByID deletes a media file from both the database and filesystem
func (s *CleanupService) DeleteFileByID(fileID int64) error {
	// Get the file first to get its path
	file, err := s.db.GetMediaFileByID(fileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file == nil {
		return fmt.Errorf("file not found: %d", fileID)
	}

	// Delete from filesystem first — if this fails, DB stays consistent
	if err := os.Remove(file.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file from filesystem: %w", err)
	}

	// Now remove from database
	if err := s.db.DeleteMediaFileByID(fileID); err != nil {
		return fmt.Errorf("failed to delete file from database (filesystem already deleted): %w", err)
	}

	return nil
}

// DeleteDuplicateFile deletes one file from a live duplicate group. It is
// idempotent for stale UI/API retries where the file has already been removed.
func (s *CleanupService) DeleteDuplicateFile(groupID string, fileID int64) error {
	analysis, err := s.AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	var targetGroup *DuplicateGroup
	for i := range analysis.Groups {
		if analysis.Groups[i].ID == groupID {
			targetGroup = &analysis.Groups[i]
			break
		}
	}

	if targetGroup == nil {
		file, err := s.db.GetMediaFileByID(fileID)
		if err != nil {
			return fmt.Errorf("failed to get file: %w", err)
		}
		if file == nil {
			return nil
		}
		return fmt.Errorf("duplicate group not found: %s", groupID)
	}

	for _, file := range targetGroup.Files {
		if file.ID == fileID {
			return s.DeleteFileByID(fileID)
		}
	}

	file, err := s.db.GetMediaFileByID(fileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file == nil {
		return nil
	}
	return fmt.Errorf("file %d is not part of duplicate group %s", fileID, groupID)
}

// DeleteDuplicateFiles deletes all files in a duplicate group except the one to keep
func (s *CleanupService) DeleteDuplicateFiles(groupID string, keepFileID int64) (int, int64, error) {
	// Find the duplicate group by ID
	analysis, err := s.AnalyzeDuplicates()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	var targetGroup *DuplicateGroup
	for i := range analysis.Groups {
		if analysis.Groups[i].ID == groupID {
			targetGroup = &analysis.Groups[i]
			break
		}
	}

	if targetGroup == nil {
		return 0, 0, fmt.Errorf("duplicate group not found: %s", groupID)
	}

	// Delete all files except the one to keep
	var deletedCount int
	var reclaimedBytes int64

	for _, file := range targetGroup.Files {
		if file.ID == keepFileID {
			continue // Keep this file
		}

		if err := s.DeleteFileByID(file.ID); err != nil {
			// Continue with other files even if one fails
			continue
		}

		deletedCount++
		reclaimedBytes += file.Size
	}

	return deletedCount, reclaimedBytes, nil
}
