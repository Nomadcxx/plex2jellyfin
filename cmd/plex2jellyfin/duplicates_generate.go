package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/plans"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func runDuplicatesGenerate() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := checkDatabasePopulated(); err != nil {
		return err
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Println("Analyzing library for duplicates...")

	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	if len(analysis.Groups) == 0 {
		fmt.Println("[ok] No duplicates found!")
		return nil
	}

	fmt.Printf("\nFound %d duplicate groups\n", len(analysis.Groups))

	// Convert to plan format
	plan := &plans.DuplicatePlan{
		Plans: make([]plans.DuplicateGroup, 0, len(analysis.Groups)),
		Summary: plans.DuplicateSummary{
			TotalGroups:      len(analysis.Groups),
			FilesToDelete:    0,
			SpaceReclaimable: 0,
		},
	}
	var skippedForReview []string

	for _, group := range analysis.Groups {
		item, reason := buildSafeDuplicatePlanEntry(group, svc, cfg)
		if reason != "" {
			skippedForReview = append(skippedForReview, fmt.Sprintf("%s: %s", group.Title, reason))
			continue
		}
		plan.Plans = append(plan.Plans, item)
		plan.Summary.FilesToDelete++
		plan.Summary.SpaceReclaimable += item.Delete.Size
	}

	if len(plan.Plans) == 0 {
		_ = plans.DeleteDuplicatePlans()
		fmt.Println("No safe duplicate deletion plan was generated.")
		for _, reason := range skippedForReview {
			fmt.Printf("  [review] %s\n", reason)
		}
		return nil
	}

	// Save plan
	if err := plans.SaveDuplicatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	fmt.Println("[ok] Duplicate plan generated")
	fmt.Printf("   Files to delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("   Space reclaimable: %s\n\n", formatBytes(plan.Summary.SpaceReclaimable))
	for _, reason := range skippedForReview {
		fmt.Printf("  [review] %s\n", reason)
	}
	fmt.Println("Next steps:")
	fmt.Println("  plex2jellyfin duplicates dry-run   # Preview deletions")
	fmt.Println("  plex2jellyfin duplicates execute   # Execute deletions")

	return nil
}

func buildSafeDuplicatePlanEntry(group service.DuplicateGroup, svc *service.CleanupService, cfg *config.Config) (plans.DuplicateGroup, string) {
	if len(group.Files) != 2 {
		return plans.DuplicateGroup{}, fmt.Sprintf("contains %d files; requires manual review", len(group.Files))
	}
	if duplicateGroupLooksUnsafeTVMovie(group, cfg) {
		return plans.DuplicateGroup{}, "movie files are under a TV library root"
	}
	if level, reasons := svc.GroupConfidence(group); level != service.ConfidenceHigh {
		return plans.DuplicateGroup{}, "low confidence: " + strings.Join(reasons, "; ")
	}

	var keepFile, deleteFile *service.MediaFile
	for i := range group.Files {
		f := &group.Files[i]
		if f.ID == group.BestFileID {
			keepFile = f
		} else {
			deleteFile = f
		}
	}
	if keepFile == nil || deleteFile == nil || keepFile.ID == deleteFile.ID {
		return plans.DuplicateGroup{}, "no unambiguous keeper and delete candidate"
	}

	fileInfo := func(file *service.MediaFile) (plans.FileInfo, string) {
		info, err := os.Stat(file.Path)
		if err != nil {
			return plans.FileInfo{}, fmt.Sprintf("cannot stat %s: %v", file.Path, err)
		}
		if !info.Mode().IsRegular() {
			return plans.FileInfo{}, fmt.Sprintf("%s is not a regular file", file.Path)
		}
		if info.Size() != file.Size {
			return plans.FileInfo{}, fmt.Sprintf("%s size changed since indexing", file.Path)
		}
		return plans.FileInfo{
			ID:              file.ID,
			Path:            file.Path,
			Size:            info.Size(),
			QualityScore:    file.QualityScore,
			Resolution:      file.Resolution,
			SourceType:      file.SourceType,
			HasStatSnapshot: true,
			ModTimeUnixNano: info.ModTime().UnixNano(),
		}, ""
	}
	keepInfo, reason := fileInfo(keepFile)
	if reason != "" {
		return plans.DuplicateGroup{}, reason
	}
	deleteInfo, reason := fileInfo(deleteFile)
	if reason != "" {
		return plans.DuplicateGroup{}, reason
	}
	return plans.DuplicateGroup{
		GroupID:   group.ID,
		MediaType: group.MediaType,
		Title:     group.Title,
		Year:      group.Year,
		Season:    group.Season,
		Episode:   group.Episode,
		Keep:      keepInfo,
		Delete:    deleteInfo,
	}, ""
}

func duplicateGroupLooksUnsafeTVMovie(group service.DuplicateGroup, cfg *config.Config) bool {
	if cfg == nil || group.MediaType != "movie" {
		return false
	}
	for _, file := range group.Files {
		for _, tvRoot := range cfg.Libraries.TV {
			if pathIsUnderRoot(file.Path, tvRoot) {
				return true
			}
		}
	}
	return false
}

func runDuplicatesDryRun() error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'plex2jellyfin duplicates generate' first to create plans.")
		return nil
	}

	fmt.Println("Duplicate Deletion Plan (DRY RUN)")
	fmt.Println()
	fmt.Printf("Files to delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n\n", formatBytes(plan.Summary.SpaceReclaimable))

	for i, p := range plan.Plans {
		yearStr := ""
		if p.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *p.Year)
		}

		if p.MediaType == "movie" {
			fmt.Printf("[%d] %s%s\n", i+1, p.Title, yearStr)
		} else {
			season := 0
			episode := 0
			if p.Season != nil {
				season = *p.Season
			}
			if p.Episode != nil {
				episode = *p.Episode
			}
			fmt.Printf("[%d] %s S%02dE%02d\n", i+1, p.Title, season, episode)
		}

		fmt.Printf("  KEEP:   %s %s (%s) - %s\n", p.Keep.Resolution, p.Keep.SourceType, formatBytes(p.Keep.Size), p.Keep.Path)
		fmt.Printf("  DELETE: %s %s (%s) - %s\n", p.Delete.Resolution, p.Delete.SourceType, formatBytes(p.Delete.Size), p.Delete.Path)
		fmt.Println()
	}

	fmt.Println("To execute: plex2jellyfin duplicates execute")
	return nil
}
