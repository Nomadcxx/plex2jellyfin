package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

type MediaType int

const (
	MediaTypeUnknown MediaType = iota
	MediaTypeMovie
	MediaTypeTVEpisode
	MediaTypeTVSeason
)

type FileInfo struct {
	Path string
	Size int64
	Name string
}

type FolderAnalysis struct {
	Path          string
	TotalSize     int64
	MainMediaFile *FileInfo
	MediaFiles    []FileInfo
	RARFiles      []FileInfo
	SampleFiles   []FileInfo
	ExtraFiles    []FileInfo
	JunkFiles     []FileInfo
	SubtitleFiles []FileInfo
	IsIncomplete  bool
	MediaType     MediaType
	DetectedTitle string
	DetectedYear  string
}

func AnalyzeFolder(path string) (*FolderAnalysis, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return analyzeFile(path)
	}

	analysis := &FolderAnalysis{
		Path: path,
	}

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		fileInfo := FileInfo{
			Path: filePath,
			Size: info.Size(),
			Name: info.Name(),
		}

		analysis.TotalSize += info.Size()
		analysis.classifyFile(fileInfo)
		return nil
	})

	if err != nil {
		return nil, err
	}

	analysis.findMainMediaFile()
	analysis.detectMediaType()
	analysis.checkCompleteness()

	return analysis, nil
}

func analyzeFile(path string) (*FolderAnalysis, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fileInfo := FileInfo{
		Path: path,
		Size: info.Size(),
		Name: info.Name(),
	}

	analysis := &FolderAnalysis{
		Path:      filepath.Dir(path),
		TotalSize: info.Size(),
	}

	if isVideoFile(fileInfo.Name) {
		analysis.MediaFiles = []FileInfo{fileInfo}
		analysis.MainMediaFile = &fileInfo
	}

	analysis.detectMediaType()
	return analysis, nil
}

func (a *FolderAnalysis) classifyFile(file FileInfo) {
	name := strings.ToLower(file.Name)
	ext := strings.ToLower(filepath.Ext(file.Name))

	switch {
	case isVideoFile(name):
		if isSampleFile(file) {
			a.SampleFiles = append(a.SampleFiles, file)
		} else if isExtraFile(name) {
			a.ExtraFiles = append(a.ExtraFiles, file)
		} else {
			a.MediaFiles = append(a.MediaFiles, file)
		}
	case isRARFile(ext):
		a.RARFiles = append(a.RARFiles, file)
	case isSubtitleFile(ext):
		a.SubtitleFiles = append(a.SubtitleFiles, file)
	case isJunkFile(ext, name):
		a.JunkFiles = append(a.JunkFiles, file)
	}
}

func (a *FolderAnalysis) findMainMediaFile() {
	if len(a.MediaFiles) == 0 {
		return
	}

	var largest *FileInfo
	for i := range a.MediaFiles {
		if largest == nil || a.MediaFiles[i].Size > largest.Size {
			largest = &a.MediaFiles[i]
		}
	}
	a.MainMediaFile = largest
}

func (a *FolderAnalysis) detectMediaType() {
	if a.MainMediaFile == nil {
		a.MediaType = MediaTypeUnknown
		return
	}

	filename := a.MainMediaFile.Name

	if naming.IsTVEpisodeFilename(filename) {
		if len(a.MediaFiles) > 1 && a.hasMultipleEpisodes() {
			a.MediaType = MediaTypeTVSeason
		} else {
			a.MediaType = MediaTypeTVEpisode
		}
		if tv, err := naming.ParseTVShowName(filename); err == nil {
			a.DetectedTitle = tv.Title
			a.DetectedYear = tv.Year
		}
		return
	}

	if naming.IsMovieFilename(filename) || a.MainMediaFile.Size > 600*1024*1024 {
		a.MediaType = MediaTypeMovie
		if movie, err := naming.ParseMovieName(filename); err == nil {
			a.DetectedTitle = movie.Title
			a.DetectedYear = movie.Year
		}
		return
	}

	if a.MainMediaFile.Size > 50*1024*1024 {
		a.MediaType = MediaTypeMovie
	} else {
		a.MediaType = MediaTypeUnknown
	}
}

func (a *FolderAnalysis) hasMultipleEpisodes() bool {
	episodeCount := 0
	for _, file := range a.MediaFiles {
		if naming.IsTVEpisodeFilename(file.Name) {
			episodeCount++
		}
	}
	return episodeCount > 1
}

func (a *FolderAnalysis) checkCompleteness() {
	if len(a.RARFiles) > 0 && len(a.MediaFiles) == 0 {
		a.IsIncomplete = true
	}
}

func (a *FolderAnalysis) HasUsableMedia() bool {
	return a.MainMediaFile != nil && !a.IsIncomplete
}

func (a *FolderAnalysis) GetCleanupFiles() []string {
	var files []string
	for _, f := range a.JunkFiles {
		files = append(files, f.Path)
	}
	for _, f := range a.SampleFiles {
		files = append(files, f.Path)
	}
	return files
}

func (a *FolderAnalysis) GetCleanupFilesPreserveExtras() []string {
	var files []string
	for _, f := range a.JunkFiles {
		files = append(files, f.Path)
	}
	for _, f := range a.SampleFiles {
		files = append(files, f.Path)
	}
	return files
}

func (a *FolderAnalysis) String() string {
	var sb strings.Builder
	sb.WriteString("Folder Analysis:\n")
	sb.WriteString(fmt.Sprintf("  Path: %s\n", a.Path))
	sb.WriteString(fmt.Sprintf("  Media Type: %s\n", a.MediaType.String()))
	if a.MainMediaFile != nil {
		sb.WriteString(fmt.Sprintf("  Main File: %s\n", a.MainMediaFile.Name))
	}
	sb.WriteString(fmt.Sprintf("  Media Files: %d\n", len(a.MediaFiles)))
	sb.WriteString(fmt.Sprintf("  Sample Files: %d\n", len(a.SampleFiles)))
	sb.WriteString(fmt.Sprintf("  RAR Files: %d\n", len(a.RARFiles)))
	sb.WriteString(fmt.Sprintf("  Junk Files: %d\n", len(a.JunkFiles)))
	if a.IsIncomplete {
		sb.WriteString("  Status: INCOMPLETE (needs extraction)\n")
	}
	return sb.String()
}

func (m MediaType) String() string {
	switch m {
	case MediaTypeMovie:
		return "Movie"
	case MediaTypeTVEpisode:
		return "TV Episode"
	case MediaTypeTVSeason:
		return "TV Season"
	default:
		return "Unknown"
	}
}

var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	".vob": true, ".divx": true, ".xvid": true,
}

func isVideoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return videoExtensions[ext]
}

var rarExtPattern = regexp.MustCompile(`^\.(rar|r\d{2})$`)

func isRARFile(ext string) bool {
	return rarExtPattern.MatchString(ext)
}

var subtitleExtensions = map[string]bool{
	".srt": true, ".sub": true, ".idx": true, ".ass": true,
	".ssa": true, ".vtt": true, ".smi": true,
}

func isSubtitleFile(ext string) bool {
	return subtitleExtensions[ext]
}

var junkExtensions = map[string]bool{
	".txt": true, ".nfo": true, ".url": true, ".html": true,
	".htm": true, ".pdf": true, ".exe": true, ".com": true,
	".bat": true, ".lnk": true, ".sfv": true, ".md5": true,
	".sha1": true, ".sha256": true, ".par2": true,
}

var junkNamePatterns = []string{
	"rarbg", "yify", "eztv", "readme", "torrent", "downloaded",
	"www.", "sample", "proof",
}

func isJunkFile(ext, name string) bool {
	if junkExtensions[ext] {
		return true
	}

	nameLower := strings.ToLower(name)
	for _, pattern := range junkNamePatterns {
		if strings.Contains(nameLower, pattern) && !isVideoFile(name) {
			return true
		}
	}

	return false
}

const sampleSizeThreshold = 100 * 1024 * 1024

var samplePatterns = regexp.MustCompile(`(?i)(^|[.\-_])sample([.\-_]|$)`)

func isSampleFile(file FileInfo) bool {
	if file.Size > sampleSizeThreshold {
		return false
	}
	return samplePatterns.MatchString(file.Name)
}

var extraPatterns = regexp.MustCompile(`(?i)(^|[.\-_])(trailer|teaser|featurette|behind.?the.?scenes|deleted.?scene|extra|bonus|interview|making.?of)([.\-_]|$)`)

func isExtraFile(name string) bool {
	return extraPatterns.MatchString(name)
}
