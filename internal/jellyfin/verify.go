package jellyfin

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IssueType represents the type of mismatch between filesystem and Jellyfin.
type IssueType string

const (
	IssueOrphaned     IssueType = "orphaned"     // Jellyfin knows about file, but file doesn't exist
	IssueMissing      IssueType = "missing"      // File exists, but Jellyfin doesn't know about it
	IssueUnidentified IssueType = "unidentified" // Jellyfin couldn't identify the file
)

// Mismatch represents a single verification issue.
type Mismatch struct {
	Path     string    `json:"path"`
	Issue    IssueType `json:"issue"`
	ItemName string    `json:"item_name,omitempty"`
	ItemID   string    `json:"item_id,omitempty"`
}

// VerificationResult contains the results of a verification scan.
type VerificationResult struct {
	LibraryPath      string      `json:"library_path"`
	ScannedCount     int         `json:"scanned_count"`
	IdentifiedCount  int         `json:"identified_count"`
	UnidentifiedCount int         `json:"unidentified_count"`
	Mismatches       []Mismatch  `json:"mismatches"`
	LastRun          time.Time   `json:"last_run"`
}

// Verifier handles verification operations between filesystem and Jellyfin.
type Verifier struct {
	client *Client
	logger Logger
}

// Logger is an optional interface for logging verification operations.
type Logger interface {
	Printf(format string, v ...interface{})
}

// NewVerifier creates a new Verifier instance.
func NewVerifier(client *Client) *Verifier {
	return &Verifier{
		client: client,
	}
}

// WithLogger sets an optional logger for the verifier.
func (v *Verifier) WithLogger(logger Logger) *Verifier {
	v.logger = logger
	return v
}

// log writes a log message if a logger is configured.
func (v *Verifier) log(format string, args ...interface{}) {
	if v.logger != nil {
		v.logger.Printf(format, args...)
	}
}

// VerifyLibrary performs a full verification of a library by ID.
func (v *Verifier) VerifyLibrary(libraryID string) (*VerificationResult, error) {
	v.log("Starting verification for library %s", libraryID)

	// Get library details
	folders, err := v.client.GetVirtualFolders()
	if err != nil {
		return nil, fmt.Errorf("getting virtual folders: %w", err)
	}

	var libraryPath string
	for _, folder := range folders {
		if folder.ItemID == libraryID {
			if len(folder.Locations) > 0 {
				libraryPath = folder.Locations[0]
			}
			break
		}
	}

	if libraryPath == "" {
		return nil, fmt.Errorf("library %s not found or has no locations", libraryID)
	}

	return v.VerifyPath(libraryPath)
}

// VerifyPath performs a full verification of a specific filesystem path.
func (v *Verifier) VerifyPath(path string) (*VerificationResult, error) {
	v.log("Starting verification for path %s", path)

	result := &VerificationResult{
		LibraryPath: path,
		LastRun:     time.Now(),
		Mismatches:  []Mismatch{},
	}

	// Get all items from Jellyfin for this path
	items, err := v.getItemsForPath(path)
	if err != nil {
		return nil, fmt.Errorf("getting items for path: %w", err)
	}

	v.log("Found %d items in Jellyfin for path %s", len(items), path)

	// Build a map of Jellyfin paths for quick lookup
	jellyfinPaths := make(map[string]Item)
	for _, item := range items {
		if item.Path != "" {
			jellyfinPaths[item.Path] = item
			result.IdentifiedCount++
		}
	}

	// Walk filesystem and find missing files
	filesystemPaths := make(map[string]bool)
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			v.log("Error accessing path %s: %v", filePath, err)
			return nil // Continue walking despite errors
		}

		if info.IsDir() {
			return nil
		}

		// Only check media files
		if !isMediaFile(filePath) {
			return nil
		}

		result.ScannedCount++
		filesystemPaths[filePath] = true

		// Check if Jellyfin knows about this file
		if _, exists := jellyfinPaths[filePath]; !exists {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:  filePath,
				Issue: IssueMissing,
			})
			v.log("Missing from Jellyfin: %s", filePath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking filesystem: %w", err)
	}

	// Check for orphaned files (in Jellyfin but not on disk)
	for path, item := range jellyfinPaths {
		if !filesystemPaths[path] {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     path,
				Issue:    IssueOrphaned,
				ItemName: item.Name,
				ItemID:   item.ID,
			})
			v.log("Orphaned in Jellyfin: %s (item: %s)", path, item.Name)
		}
	}

	v.log("Verification complete: scanned=%d, identified=%d, mismatches=%d",
		result.ScannedCount, result.IdentifiedCount, len(result.Mismatches))

	return result, nil
}

// GetUnidentifiedItems returns items that Jellyfin couldn't identify.
func (v *Verifier) GetUnidentifiedItems(libraryID string) ([]Mismatch, error) {
	v.log("Getting unidentified items for library %s", libraryID)

	// Query for items with no external IDs
	query := url.Values{}
	query.Set("ParentId", libraryID)
	query.Set("Recursive", "true")
	query.Set("HasTmdbId", "false")
	query.Set("HasTvdbId", "false")
	query.Set("HasImdbId", "false")

	var resp ItemsResponse
	if err := v.client.get("/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("querying unidentified items: %w", err)
	}

	v.log("Found %d unidentified items", len(resp.Items))

	mismatches := make([]Mismatch, 0, len(resp.Items))
	for _, item := range resp.Items {
		// Skip folders/seasons, only report actual media
		if item.Type == "Folder" || item.Type == "Season" || item.Type == "Series" {
			continue
		}

		mismatches = append(mismatches, Mismatch{
			Path:     item.Path,
			Issue:    IssueUnidentified,
			ItemName: item.Name,
			ItemID:   item.ID,
		})
	}

	v.log("Returning %d unidentified media items", len(mismatches))
	return mismatches, nil
}

// FindOrphanedFiles finds files that Jellyfin knows about but don't exist on disk.
func (v *Verifier) FindOrphanedFiles(libraryID string) ([]Mismatch, error) {
	v.log("Finding orphaned files for library %s", libraryID)

	// Get all items for this library
	items, err := v.getItemsByLibrary(libraryID)
	if err != nil {
		return nil, fmt.Errorf("getting library items: %w", err)
	}

	v.log("Checking %d items for orphaned files", len(items))

	orphaned := []Mismatch{}
	for _, item := range items {
		if item.Path == "" {
			continue
		}

		// Check if file exists
		if _, err := os.Stat(item.Path); os.IsNotExist(err) {
			orphaned = append(orphaned, Mismatch{
				Path:     item.Path,
				Issue:    IssueOrphaned,
				ItemName: item.Name,
				ItemID:   item.ID,
			})
			v.log("Orphaned: %s (item: %s)", item.Path, item.Name)
		}
	}

	v.log("Found %d orphaned files", len(orphaned))
	return orphaned, nil
}

// FindMissingFromJellyfin finds files that exist on disk but aren't in Jellyfin.
func (v *Verifier) FindMissingFromJellyfin(libraryPath string) ([]Mismatch, error) {
	v.log("Finding missing files for library path %s", libraryPath)

	// Check if path exists
	if _, err := os.Stat(libraryPath); err != nil {
		return nil, fmt.Errorf("accessing library path: %w", err)
	}

	// Get all items from Jellyfin for this path
	items, err := v.getItemsForPath(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("getting jellyfin items: %w", err)
	}

	v.log("Found %d items in Jellyfin", len(items))

	// Build set of known paths
	jellyfinPaths := make(map[string]bool)
	for _, item := range items {
		if item.Path != "" {
			jellyfinPaths[item.Path] = true
		}
	}

	// Walk filesystem
	missing := []Mismatch{}
	err = filepath.Walk(libraryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			v.log("Error accessing %s: %v", path, err)
			return nil // Continue despite errors
		}

		if info.IsDir() {
			return nil
		}

		// Only check media files
		if !isMediaFile(path) {
			return nil
		}

		// Check if Jellyfin knows about this file
		if !jellyfinPaths[path] {
			missing = append(missing, Mismatch{
				Path:  path,
				Issue: IssueMissing,
			})
			v.log("Missing from Jellyfin: %s", path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking filesystem: %w", err)
	}

	v.log("Found %d missing files", len(missing))
	return missing, nil
}

// getItemsForPath retrieves all items from Jellyfin that are under a specific path.
func (v *Verifier) getItemsForPath(path string) ([]Item, error) {
	// Get all virtual folders
	folders, err := v.client.GetVirtualFolders()
	if err != nil {
		return nil, fmt.Errorf("getting virtual folders: %w", err)
	}

	// Find matching library
	var libraryID string
	for _, folder := range folders {
		for _, location := range folder.Locations {
			if strings.HasPrefix(path, location) || location == path {
				libraryID = folder.ItemID
				break
			}
		}
		if libraryID != "" {
			break
		}
	}

	if libraryID == "" {
		return nil, fmt.Errorf("no library found for path %s", path)
	}

	return v.getItemsByLibrary(libraryID)
}

// getItemsByLibrary retrieves all items for a library ID.
func (v *Verifier) getItemsByLibrary(libraryID string) ([]Item, error) {
	query := url.Values{}
	query.Set("ParentId", libraryID)
	query.Set("Recursive", "true")
	query.Set("Fields", "Path")

	var resp ItemsResponse
	if err := v.client.get("/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("querying items: %w", err)
	}

	return resp.Items, nil
}

// isMediaFile checks if a file is a media file based on extension.
func isMediaFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	mediaExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".m4v":  true,
		".webm": true,
		".mpg":  true,
		".mpeg": true,
		".m2ts": true,
		".ts":   true,
	}
	return mediaExts[ext]
}
