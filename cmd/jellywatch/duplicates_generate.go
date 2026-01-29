package main

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/service"
)

func runDuplicatesGenerate() error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Println("🔍 Analyzing library for duplicates...")

	svc := service.NewCleanupService(db)
	analysis, err := svc.AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	if len(analysis.Groups) == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	fmt.Printf("\nFound %d duplicate groups\n", len(analysis.Groups))
	fmt.Printf("Total space reclaimable: %s\n\n", formatBytes(analysis.ReclaimableBytes))

	// Convert to plan format
	plan := &plans.DuplicatePlan{
		Plans: make([]plans.DuplicateGroup, 0, len(analysis.Groups)),
		Summary: plans.DuplicateSummary{
			TotalGroups:      len(analysis.Groups),
			FilesToDelete:    0,
			SpaceReclaimable: analysis.ReclaimableBytes,
		},
	}

	for _, group := range analysis.Groups {
		if len(group.Files) < 2 {
			continue
		}

		var keepFile, deleteFile *service.MediaFile
		for i := range group.Files {
			f := &group.Files[i]
			if f.ID == group.BestFileID {
				keepFile = f
			} else if deleteFile == nil {
				deleteFile = f
			}
		}

		if keepFile == nil || deleteFile == nil {
			continue
		}

		item := plans.DuplicateGroup{
			GroupID:   group.ID,
			MediaType: group.MediaType,
			Title:     group.Title,
			Year:      group.Year,
			Season:    group.Season,
			Episode:   group.Episode,
			Keep: plans.FileInfo{
				ID:           keepFile.ID,
				Path:         keepFile.Path,
				Size:         keepFile.Size,
				Resolution:   keepFile.Resolution,
				SourceType:   keepFile.SourceType,
				QualityScore: keepFile.QualityScore,
			},
			Delete: plans.FileInfo{
				ID:           deleteFile.ID,
				Path:         deleteFile.Path,
				Size:         deleteFile.Size,
				Resolution:   deleteFile.Resolution,
				SourceType:   deleteFile.SourceType,
				QualityScore: deleteFile.QualityScore,
			},
		}

		plan.Plans = append(plan.Plans, item)
		plan.Summary.FilesToDelete++
	}

	// Save plan
	if err := plans.SaveDuplicatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	fmt.Println("✅ Duplicate plan generated")
	fmt.Printf("   Files to delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("   Space reclaimable: %s\n\n", formatBytes(plan.Summary.SpaceReclaimable))
	fmt.Println("Next steps:")
	fmt.Println("  jellywatch duplicates dry-run   # Preview deletions")
	fmt.Println("  jellywatch duplicates execute   # Execute deletions")

	return nil
}

func runDuplicatesDryRun() error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates generate' first to create plans.")
		return nil
	}

	fmt.Println("📋 Duplicate Deletion Plan (DRY RUN)")
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

	fmt.Println("To execute: jellywatch duplicates execute")
	return nil
}
