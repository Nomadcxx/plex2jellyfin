package organizer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/analyzer"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/library"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/quality"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

type OrganizationResult struct {
	Success         bool
	SourcePath      string
	TargetPath      string
	BytesCopied     int64
	Duration        time.Duration
	Attempts        int
	Error           error
	Skipped         bool
	SkipReason      string
	SourceQuality   *quality.QualityInfo
	ExistingQuality *quality.QualityInfo
}

type Organizer struct {
	dryRun         bool
	keepSource     bool
	forceOverwrite bool
	selector       *library.Selector
	transferer     transfer.Transferer
	timeout        time.Duration
	checksumVerify bool
	targetUID      int
	targetGID      int
	fileMode       os.FileMode
	dirMode        os.FileMode
	sonarrClient   *sonarr.Client
	db             *database.MediaDB // HOLDEN: Database for self-learning
}

func NewOrganizer(libraries []string, options ...func(*Organizer)) (*Organizer, error) {
	transferer, err := transfer.New(transfer.BackendAuto)
	if err != nil {
		return nil, fmt.Errorf("failed to create transferer: %w", err)
	}

	org := &Organizer{
		dryRun:         false,
		keepSource:     false,
		forceOverwrite: false,
		transferer:     transferer,
		timeout:        5 * time.Minute,
		checksumVerify: false,
		targetUID:      -1,
		targetGID:      -1,
		fileMode:       0,
		dirMode:        0,
	}

	// Apply options first (to get sonarrClient and db if provided)
	for _, opt := range options {
		opt(org)
	}

	// Create selector with Sonarr and database integration if available
	org.selector = library.NewSelectorWithConfig(library.SelectorConfig{
		Libraries:     libraries,
		SonarrClient:  org.sonarrClient,
		CacheDuration: 5 * time.Minute,
		DB:            org.db,
	})

	return org, nil
}

// WithDryRun sets dry run mode
func WithDryRun(dryRun bool) func(*Organizer) {
	return func(o *Organizer) {
		o.dryRun = dryRun
	}
}

// WithKeepSource sets whether to keep source files
func WithKeepSource(keep bool) func(*Organizer) {
	return func(o *Organizer) {
		o.keepSource = keep
	}
}

// WithTimeout sets the transfer timeout
func WithTimeout(timeout time.Duration) func(*Organizer) {
	return func(o *Organizer) {
		o.timeout = timeout
	}
}

func WithChecksumVerify(verify bool) func(*Organizer) {
	return func(o *Organizer) {
		o.checksumVerify = verify
	}
}

func WithBackend(backend transfer.Backend) func(*Organizer) {
	return func(o *Organizer) {
		t, err := transfer.New(backend)
		if err != nil {
			return
		}
		o.transferer = t
	}
}

func WithForceOverwrite(force bool) func(*Organizer) {
	return func(o *Organizer) {
		o.forceOverwrite = force
	}
}

func WithPermissions(uid, gid int, fileMode, dirMode os.FileMode) func(*Organizer) {
	return func(o *Organizer) {
		o.targetUID = uid
		o.targetGID = gid
		o.fileMode = fileMode
		o.dirMode = dirMode
	}
}

// WithSonarrClient sets the Sonarr client for intelligent library selection
func WithSonarrClient(client *sonarr.Client) func(*Organizer) {
	return func(o *Organizer) {
		o.sonarrClient = client
	}
}

// WithDatabase sets the HOLDEN database for self-learning
func WithDatabase(db *database.MediaDB) func(*Organizer) {
	return func(o *Organizer) {
		o.db = db
	}
}

func (o *Organizer) buildTransferOptions() transfer.TransferOptions {
	return transfer.TransferOptions{
		Timeout:       o.timeout,
		Checksum:      o.checksumVerify,
		RetryAttempts: 3,
		RetryDelay:    5 * time.Second,
		PreserveAttrs: true,
		TargetUID:     o.targetUID,
		TargetGID:     o.targetGID,
		FileMode:      o.fileMode,
		DirMode:       o.dirMode,
	}
}

func (o *Organizer) applyDirOwnership(path string) error {
	if o.dirMode != 0 {
		if err := os.Chmod(path, o.dirMode); err != nil {
			return err
		}
	}
	if o.targetUID >= 0 || o.targetGID >= 0 {
		uid, gid := o.targetUID, o.targetGID
		if uid < 0 {
			uid = -1
		}
		if gid < 0 {
			gid = -1
		}
		if err := os.Chown(path, uid, gid); err != nil {
			if os.Geteuid() != 0 {
				return nil
			}
			return err
		}
	}
	return nil
}

func (o *Organizer) OrganizeMovie(sourcePath, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	movie, err := naming.ParseMovieFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse movie name: %w", err),
		}, nil
	}

	cleanName := naming.NormalizeMovieName(movie.Title, movie.Year)
	movieDir := filepath.Join(libraryPath, cleanName)
	ext := filepath.Ext(sourcePath)
	targetPath := filepath.Join(movieDir, cleanName+ext)

	existingFile, existingQuality := o.findExistingMediaFile(movieDir)
	if existingFile != "" && !o.forceOverwrite {
		if !sourceQuality.IsBetterThan(existingQuality) {
			return &OrganizationResult{
				Success:         false,
				SourcePath:      sourcePath,
				TargetPath:      existingFile,
				Skipped:         true,
				SkipReason:      fmt.Sprintf("existing file has equal or better quality (%s vs %s)", existingQuality.String(), sourceQuality.String()),
				SourceQuality:   sourceQuality,
				ExistingQuality: existingQuality,
			}, nil
		}
	}

	if err := os.MkdirAll(movieDir, 0755); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to create directory: %w", err),
		}, nil
	}

	if err := o.applyDirOwnership(movieDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set directory permissions: %w", err),
		}, nil
	}

	if o.dryRun {
		return &OrganizationResult{
			Success:         true,
			SourcePath:      sourcePath,
			TargetPath:      targetPath,
			SourceQuality:   sourceQuality,
			ExistingQuality: existingQuality,
		}, nil
	}

	if existingFile != "" && existingFile != targetPath {
		os.Remove(existingFile)
	}

	opts := o.buildTransferOptions()

	var result *transfer.TransferResult
	if o.keepSource {
		result, err = o.transferer.Copy(sourcePath, targetPath, opts)
	} else {
		result, err = o.transferer.Move(sourcePath, targetPath, opts)
	}

	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			BytesCopied:   result.BytesCopied,
			Duration:      result.Duration,
			Attempts:      result.Attempts,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("transfer failed: %w", err),
		}, nil
	}

	// HOLDEN Phase 3: Self-learning - update database with organized movie
	if o.db != nil {
		yearInt := 0
		if movie.Year != "" {
			fmt.Sscanf(movie.Year, "%d", &yearInt)
		}
		movieRecord := &database.Movie{
			Title:          movie.Title,
			Year:           yearInt,
			CanonicalPath:  movieDir,
			LibraryRoot:    libraryPath,
			Source:         "jellywatch",
			SourcePriority: 100, // Highest priority
		}
		_, _ = o.db.UpsertMovie(movieRecord)
		// Ignore errors - database update is best-effort
	}

	return &OrganizationResult{
		Success:         true,
		SourcePath:      sourcePath,
		TargetPath:      targetPath,
		BytesCopied:     result.BytesCopied,
		Duration:        result.Duration,
		Attempts:        result.Attempts,
		SourceQuality:   sourceQuality,
		ExistingQuality: existingQuality,
	}, nil
}

func (o *Organizer) OrganizeTVEpisode(sourcePath, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil
	}

	showDir := findExistingShowDir(libraryPath, tv.Title)
	if showDir == "" {
		showName := naming.NormalizeTVShowName(tv.Title, tv.Year)
		showDir = filepath.Join(libraryPath, showName)
	}

	seasonDir := filepath.Join(showDir, naming.FormatSeasonFolder(tv.Season))
	ext := filepath.Ext(sourcePath)
	episodeName := naming.FormatTVEpisodeFilename(tv.Title, tv.Year, tv.Season, tv.Episode, ext[1:])
	targetPath := filepath.Join(seasonDir, episodeName)

	existingFile, existingQuality := o.findExistingMediaFile(seasonDir)
	if existingFile != "" && !o.forceOverwrite {
		existingBase := filepath.Base(existingFile)
		targetBase := filepath.Base(targetPath)
		sameEpisode := strings.HasPrefix(existingBase, strings.TrimSuffix(targetBase, ext))
		if sameEpisode && !sourceQuality.IsBetterThan(existingQuality) {
			return &OrganizationResult{
				Success:         false,
				SourcePath:      sourcePath,
				TargetPath:      existingFile,
				Skipped:         true,
				SkipReason:      fmt.Sprintf("existing file has equal or better quality (%s vs %s)", existingQuality.String(), sourceQuality.String()),
				SourceQuality:   sourceQuality,
				ExistingQuality: existingQuality,
			}, nil
		}
	}

	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to create directories: %w", err),
		}, nil
	}

	if err := o.applyDirOwnership(showDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set show directory permissions: %w", err),
		}, nil
	}
	if err := o.applyDirOwnership(seasonDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set season directory permissions: %w", err),
		}, nil
	}

	if o.dryRun {
		return &OrganizationResult{
			Success:         true,
			SourcePath:      sourcePath,
			TargetPath:      targetPath,
			SourceQuality:   sourceQuality,
			ExistingQuality: existingQuality,
		}, nil
	}

	if existingFile != "" && existingFile != targetPath {
		os.Remove(existingFile)
	}

	opts := o.buildTransferOptions()

	var result *transfer.TransferResult
	if o.keepSource {
		result, err = o.transferer.Copy(sourcePath, targetPath, opts)
	} else {
		result, err = o.transferer.Move(sourcePath, targetPath, opts)
	}

	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			BytesCopied:   result.BytesCopied,
			Duration:      result.Duration,
			Attempts:      result.Attempts,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("transfer failed: %w", err),
		}, nil
	}

	// HOLDEN Phase 3: Self-learning - update database with organized TV show
	if o.db != nil {
		yearInt := 0
		if tv.Year != "" {
			fmt.Sscanf(tv.Year, "%d", &yearInt)
		}
		seriesRecord := &database.Series{
			Title:          tv.Title,
			Year:           yearInt,
			CanonicalPath:  showDir,
			LibraryRoot:    libraryPath,
			Source:         "jellywatch",
			SourcePriority: 100, // Highest priority
			EpisodeCount:   0,   // Will be updated by filesystem sync
		}
		_, _ = o.db.UpsertSeries(seriesRecord)
		// Ignore errors - database update is best-effort
	}

	return &OrganizationResult{
		Success:         true,
		SourcePath:      sourcePath,
		TargetPath:      targetPath,
		BytesCopied:     result.BytesCopied,
		Duration:        result.Duration,
		Attempts:        result.Attempts,
		SourceQuality:   sourceQuality,
		ExistingQuality: existingQuality,
	}, nil
}

func (o *Organizer) OrganizeTVEpisodeAuto(sourcePath string, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	info, err := getFileSize(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil
	}

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil
	}

	selection, err := o.selector.SelectTVShowLibrary(tv.Title, tv.Year, info)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to select library: %w", err),
		}, nil
	}

	return o.OrganizeTVEpisode(sourcePath, selection.Library)
}

// findExistingMediaFile scans a directory for existing video files and returns
// the path and quality info of the best existing file.
// Returns ("", nil) if no video files found.
func (o *Organizer) findExistingMediaFile(dir string) (string, *quality.QualityInfo) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}

	videoExtensions := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".wmv": true, ".m4v": true, ".ts": true, ".m2ts": true,
	}

	var bestPath string
	var bestQuality *quality.QualityInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !videoExtensions[ext] {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		fileQuality := quality.Parse(entry.Name())

		if bestQuality == nil || fileQuality.IsBetterThan(bestQuality) {
			bestPath = filePath
			bestQuality = fileQuality
		}
	}

	return bestPath, bestQuality
}

func findExistingShowDir(libraryPath, showTitle string) string {
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return ""
	}

	titleLower := strings.ToLower(showTitle)
	yearPattern := regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		dirNameLower := strings.ToLower(dirName)

		if dirNameLower == titleLower {
			return filepath.Join(libraryPath, dirName)
		}

		baseName := yearPattern.ReplaceAllString(dirName, "")
		if strings.ToLower(baseName) == titleLower {
			return filepath.Join(libraryPath, dirName)
		}
	}

	return ""
}

type FolderOrganizationResult struct {
	Analysis        *analyzer.FolderAnalysis
	MediaResult     *OrganizationResult
	SubtitlesCopied []string
	JunkRemoved     []string
	SamplesRemoved  []string
	ExtrasSkipped   []string
	Error           error
}

func (o *Organizer) OrganizeFolder(folderPath, libraryPath string, keepExtras bool) (*FolderOrganizationResult, error) {
	analysis, err := analyzer.AnalyzeFolder(folderPath)
	if err != nil {
		return &FolderOrganizationResult{Error: err}, err
	}

	result := &FolderOrganizationResult{
		Analysis: analysis,
	}

	if !analysis.HasUsableMedia() {
		result.Error = fmt.Errorf("no usable media found in folder (incomplete or empty)")
		return result, nil
	}

	switch analysis.MediaType {
	case analyzer.MediaTypeMovie:
		mediaResult, err := o.OrganizeMovie(analysis.MainMediaFile.Path, libraryPath)
		result.MediaResult = mediaResult
		if err != nil || !mediaResult.Success {
			return result, err
		}

		if !o.dryRun {
			result.SubtitlesCopied = o.copySubtitles(analysis, filepath.Dir(mediaResult.TargetPath))
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
			if !keepExtras {
				result.ExtrasSkipped = o.getFilePaths(analysis.ExtraFiles)
			}
		}

	case analyzer.MediaTypeTVEpisode:
		mediaResult, err := o.OrganizeTVEpisode(analysis.MainMediaFile.Path, libraryPath)
		result.MediaResult = mediaResult
		if err != nil || !mediaResult.Success {
			return result, err
		}

		if !o.dryRun {
			result.SubtitlesCopied = o.copySubtitles(analysis, filepath.Dir(mediaResult.TargetPath))
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
		}

	case analyzer.MediaTypeTVSeason:
		for _, mediaFile := range analysis.MediaFiles {
			mediaResult, err := o.OrganizeTVEpisode(mediaFile.Path, libraryPath)
			if err != nil {
				result.Error = err
				return result, err
			}
			result.MediaResult = mediaResult
		}

		if !o.dryRun {
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
		}

	default:
		result.Error = fmt.Errorf("unknown media type")
		return result, nil
	}

	return result, nil
}

func (o *Organizer) copySubtitles(analysis *analyzer.FolderAnalysis, targetDir string) []string {
	var copied []string
	for _, sub := range analysis.SubtitleFiles {
		targetPath := filepath.Join(targetDir, sub.Name)
		opts := transfer.TransferOptions{
			Timeout:       30 * time.Second,
			RetryAttempts: 2,
			TargetUID:     o.targetUID,
			TargetGID:     o.targetGID,
			FileMode:      o.fileMode,
		}
		_, err := o.transferer.Copy(sub.Path, targetPath, opts)
		if err == nil {
			copied = append(copied, sub.Name)
		}
	}
	return copied
}

func (o *Organizer) removeFiles(files []analyzer.FileInfo) []string {
	var removed []string
	for _, f := range files {
		if err := os.Remove(f.Path); err == nil {
			removed = append(removed, f.Name)
		}
	}
	return removed
}

func (o *Organizer) getFilePaths(files []analyzer.FileInfo) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Name
	}
	return paths
}
