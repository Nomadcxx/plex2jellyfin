package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/naming"
)

type MediaFile struct {
	Path      string
	MediaType string // "movie" or "episode" from database
}

type TestResult struct {
	Path             string
	DBClassification string
	SourceUnknown    bool
	SourceTV         bool
	SourceMovie      bool
	CorrectHint      naming.SourceHint
	WithCorrectHint  bool
	IsCorrect        bool
}

func main() {
	// Read media files from CSV
	files, err := readMediaFiles("/tmp/media_files_test.csv")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading media files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Testing %d media files from database...\n\n", len(files))

	var results []TestResult
	var correctCount, misclassificationCount int

	for i, file := range files {
		if i%1000 == 0 {
			fmt.Printf("Progress: %d/%d files tested\n", i, len(files))
		}

		result := testFile(file)
		results = append(results, result)

		if result.IsCorrect {
			correctCount++
		} else {
			misclassificationCount++
		}
	}

	fmt.Printf("\n=== RESULTS ===\n")
	fmt.Printf("Total files: %d\n", len(files))
	fmt.Printf("Correct classifications: %d (%.2f%%)\n", correctCount, float64(correctCount)/float64(len(files))*100)
	fmt.Printf("Misclassifications: %d (%.2f%%)\n", misclassificationCount, float64(misclassificationCount)/float64(len(files))*100)

	// Print misclassifications
	if misclassificationCount > 0 {
		fmt.Printf("\n=== MISCLASSIFICATIONS ===\n")
		for _, result := range results {
			if !result.IsCorrect {
				fmt.Printf("\nPath: %s\n", result.Path)
				fmt.Printf("  DB classification: %s\n", result.DBClassification)
				fmt.Printf("  Correct hint: %v\n", result.CorrectHint)
				fmt.Printf("  With correct hint: %v\n", result.WithCorrectHint)
				fmt.Printf("  IsTVEpisodeFromPath(SourceUnknown): %v\n", result.SourceUnknown)
				fmt.Printf("  IsTVEpisodeFromPath(SourceTV): %v\n", result.SourceTV)
				fmt.Printf("  IsTVEpisodeFromPath(SourceMovie): %v\n", result.SourceMovie)
			}
		}
	}

	// Analyze patterns
	fmt.Printf("\n=== PATTERN ANALYSIS ===\n")
	analyzePatterns(results)
}

func readMediaFiles(filename string) ([]MediaFile, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var files []MediaFile
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// Format: path|media_type
		parts := strings.Split(line, "|")
		if len(parts) == 2 {
			files = append(files, MediaFile{
				Path:      parts[0],
				MediaType: parts[1],
			})
		}
	}

	return files, scanner.Err()
}

func testFile(file MediaFile) TestResult {
	result := TestResult{
		Path:             file.Path,
		DBClassification: file.MediaType,
	}

	result.SourceUnknown = naming.IsTVEpisodeFromPath(file.Path, naming.SourceUnknown)
	result.SourceTV = naming.IsTVEpisodeFromPath(file.Path, naming.SourceTV)
	result.SourceMovie = naming.IsTVEpisodeFromPath(file.Path, naming.SourceMovie)

	pathUpper := strings.ToUpper(file.Path)
	if strings.Contains(pathUpper, "TVSHOWS") {
		result.CorrectHint = naming.SourceTV
		result.WithCorrectHint = naming.IsTVEpisodeFromPath(file.Path, naming.SourceTV)
		result.IsCorrect = (result.WithCorrectHint == true)
	} else if strings.Contains(pathUpper, "MOVIES") {
		result.CorrectHint = naming.SourceMovie
		result.WithCorrectHint = naming.IsTVEpisodeFromPath(file.Path, naming.SourceMovie)
		result.IsCorrect = (result.WithCorrectHint == false)
	} else {
		result.CorrectHint = naming.SourceUnknown
		expectedIsTV := (file.MediaType == "episode")
		result.WithCorrectHint = naming.IsTVEpisodeFromPath(file.Path, naming.SourceUnknown)
		result.IsCorrect = (result.WithCorrectHint == expectedIsTV)
	}

	return result
}

func analyzePatterns(results []TestResult) {
	tvShowsCount := 0
	moviesCount := 0
	otherCount := 0

	tvShowsCorrect := 0
	moviesCorrect := 0
	otherCorrect := 0

	for _, result := range results {
		pathUpper := strings.ToUpper(result.Path)

		if strings.Contains(pathUpper, "TVSHOWS") {
			tvShowsCount++
			if result.IsCorrect {
				tvShowsCorrect++
			}
		} else if strings.Contains(pathUpper, "MOVIES") {
			moviesCount++
			if result.IsCorrect {
				moviesCorrect++
			}
		} else {
			otherCount++
			if result.IsCorrect {
				otherCorrect++
			}
		}
	}

	fmt.Printf("\nFiles in TVSHOWS directories: %d (correct: %d, %.2f%%)\n",
		tvShowsCount, tvShowsCorrect, float64(tvShowsCorrect)/float64(tvShowsCount)*100)
	fmt.Printf("Files in MOVIES directories: %d (correct: %d, %.2f%%)\n",
		moviesCount, moviesCorrect, float64(moviesCorrect)/float64(moviesCount)*100)
	fmt.Printf("Files in other directories: %d (correct: %d, %.2f%%)\n",
		otherCount, otherCorrect, float64(otherCorrect)/float64(otherCount)*100)

	fmt.Printf("\n=== DAILY SHOW PATTERNS ===\n")
	dailyShowCount := 0
	for _, result := range results {
		if containsDatePattern(result.Path) {
			dailyShowCount++
			if dailyShowCount <= 10 {
				fmt.Printf("Date pattern found: %s\n", filepath.Base(result.Path))
				fmt.Printf("  DB classification: %s\n", result.DBClassification)
				fmt.Printf("  With correct hint: %v\n", result.WithCorrectHint)
			}
		}
	}
	if dailyShowCount > 10 {
		fmt.Printf("... and %d more files with date patterns\n", dailyShowCount-10)
	}
	fmt.Printf("Total files with date patterns: %d\n", dailyShowCount)
}

func containsDatePattern(path string) bool {
	// Check for YYYY.MM.DD or YYYY-MM-DD patterns in filename
	filename := filepath.Base(path)
	// YYYY.MM.DD pattern
	if strings.Contains(filename, "2025.") || strings.Contains(filename, "2024.") ||
		strings.Contains(filename, "2023.") || strings.Contains(filename, "2022.") {
		// Verify it's followed by MM.DD
		parts := strings.Split(filename, "2025.")
		for _, part := range parts {
			if len(part) >= 5 && part[2] == '.' && part[5] == '.' {
				return true
			}
		}
		parts = strings.Split(filename, "2024.")
		for _, part := range parts {
			if len(part) >= 5 && part[2] == '.' && part[5] == '.' {
				return true
			}
		}
		parts = strings.Split(filename, "2023.")
		for _, part := range parts {
			if len(part) >= 5 && part[2] == '.' && part[5] == '.' {
				return true
			}
		}
	}
	return false
}
