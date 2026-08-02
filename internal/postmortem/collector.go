package postmortem

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

type Collector struct {
	DB             *database.MediaDB
	Root           string
	Since          time.Time
	Now            func() time.Time
	LogDir         string
	Workspace      string
	UnknownSeasons func() UnknownSeasonEvidence
}

var journalctlExcerpt = func(since time.Time) (string, error) {
	if since.IsZero() {
		since = time.Now().Add(-96 * time.Hour)
	}
	args := journalctlDaemonArgs(since)
	out, err := exec.Command("journalctl", args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("%w: %s", err, text)
		}
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("journalctl returned no lines")
	}
	return text, nil
}

func journalctlDaemonArgs(since time.Time) []string {
	return []string{
		"-u", "plex2jellyfin-daemon",
		"--since", since.Local().Format("2006-01-02 15:04:05"),
		"--no-pager",
	}
}

type housekeepingSnapshot struct {
	Counts          map[string]int              `json:"counts"` // alias of outstanding for older consumers
	CreatedInWindow map[string]int              `json:"created_in_window"`
	Outstanding     map[string]int              `json:"outstanding"`
	Recent          []database.HousekeepingTask `json:"recent"`
	Error           string                      `json:"error,omitempty"`
}

type jellyfinDiffSnapshot struct {
	PathTranslationFalsePositives []SuspiciousItem `json:"path_translation_false_positives"`
}

func (c Collector) Collect() (BundlePaths, error) {
	if c.DB == nil {
		return BundlePaths{}, fmt.Errorf("database is required")
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	if c.Since.IsZero() {
		c.Since = now.Add(-96 * time.Hour)
	}
	if c.Root == "" {
		return BundlePaths{}, fmt.Errorf("report root is required")
	}
	if c.Workspace == "" {
		c.Workspace = "/home/nomadx/Documents/plex2jellyfin"
	}

	bundle := NewBundlePaths(c.Root, RunID(now))
	if err := os.MkdirAll(bundle.Dir, 0o755); err != nil {
		return BundlePaths{}, fmt.Errorf("create report dir: %w", err)
	}

	decisions, err := c.DB.QueryDecisions(database.QueryFilter{EventAfter: &c.Since, Limit: 10000})
	if err != nil {
		return BundlePaths{}, fmt.Errorf("query parse decisions: %w", err)
	}
	repairs, err := c.DB.ListRepairEventsSince(c.Since, 10000)
	if err != nil {
		return BundlePaths{}, fmt.Errorf("query repair events: %w", err)
	}
	hk := c.housekeeping()
	unknownSeasons := c.unknownSeasonEvidence()
	suspicious, pathFalsePositives := suspiciousFromDecisions(decisions)
	metrics := SummarizeDecisionMetrics(decisions, now)
	summary := Summary{
		RunID:                         bundle.RunID,
		GeneratedAt:                   now,
		Since:                         c.Since,
		ProcessedDecisions:            metrics.ProcessedDecisions,
		RepairEvents:                  len(repairs),
		SuspiciousItems:               len(suspicious),
		MetadataProblems:              metrics.MetadataProblems,
		DriftLabels:                   metrics.DriftLabels,
		FailLabels:                    metrics.FailLabels,
		PendingLabels:                 metrics.PendingLabels,
		OverdueUnlabeled:              metrics.OverdueUnlabeled,
		HousekeepingFailed:            hk.CreatedInWindow[database.TaskStatusFailed],
		ManualReview:                  hk.CreatedInWindow[database.TaskStatusFlagged],
		HousekeepingOutstandingFailed: hk.Outstanding[database.TaskStatusFailed],
		HousekeepingOutstandingReview: hk.Outstanding[database.TaskStatusFlagged],
		UnknownSeasonActionable:       unknownSeasons.ActionablePollutionEpisodes,
	}

	if err := writeJSON(bundle.File("summary.json"), summary); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("repair-events.json"), repairs); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("jellyfin-diff.json"), jellyfinDiffSnapshot{PathTranslationFalsePositives: pathFalsePositives}); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("parse-decisions.json"), parseDecisionEvidenceList(decisions)); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("housekeeping.json"), hk); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("suspicious-items.json"), suspicious); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("unknown-seasons.json"), unknownSeasons); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("media-inventory.json"), c.mediaInventory()); err != nil {
		return bundle, err
	}
	if err := writeJSON(bundle.File("config-snapshot.json"), c.configSnapshot()); err != nil {
		return bundle, err
	}
	if err := writeText(bundle.File("daemon-log-excerpt.txt"), c.daemonLogExcerpt()); err != nil {
		return bundle, err
	}
	if err := writeText(bundle.File("context.md"), ContextMarkdown()); err != nil {
		return bundle, err
	}
	if err := writeText(bundle.File("agent-prompt.md"), AgentPrompt(c.Workspace, bundle.LatestLink)); err != nil {
		return bundle, err
	}
	if err := writeText(bundle.File("report.md"), MarkdownReport(summary, suspicious, unknownSeasons)); err != nil {
		return bundle, err
	}
	if err := updateLatestLink(bundle); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func (c Collector) unknownSeasonEvidence() UnknownSeasonEvidence {
	if c.UnknownSeasons != nil {
		return c.UnknownSeasons()
	}
	cfg, err := config.Load()
	if err != nil {
		return UnknownSeasonEvidence{Error: err.Error()}
	}
	if strings.TrimSpace(cfg.Jellyfin.URL) == "" || strings.TrimSpace(cfg.Jellyfin.APIKey) == "" {
		return UnknownSeasonEvidence{Error: "jellyfin url/api_key not configured"}
	}
	client := jellyfin.NewClient(jellyfin.Config{
		URL:     cfg.Jellyfin.URL,
		APIKey:  cfg.Jellyfin.APIKey,
		Timeout: 30 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := client.AuditUnknownSeasons(ctx, "")
	if err != nil {
		return UnknownSeasonEvidence{Error: err.Error()}
	}
	return unknownSeasonEvidenceFromReport(report)
}

func unknownSeasonEvidenceFromReport(report *jellyfin.UnknownSeasonReport) UnknownSeasonEvidence {
	if report == nil {
		return UnknownSeasonEvidence{Error: "unknown season report unavailable"}
	}
	actionable := report.RefreshCandidateEpisodes + report.RandomishBasenameEpisodes
	return UnknownSeasonEvidence{
		UserID:                      report.UserID,
		Total:                       report.Total,
		RefreshRepairableSeasons:    report.RefreshRepairableSeasons,
		RefreshRepairableEpisodes:   report.RefreshRepairableEpisodes,
		RefreshCandidateSeasons:     report.RefreshCandidateSeasons,
		RefreshCandidateEpisodes:    report.RefreshCandidateEpisodes,
		RandomishBasenameEpisodes:   report.RandomishBasenameEpisodes,
		ActionablePollutionEpisodes: actionable,
		FolderContext:               report.FolderContext,
		MixedReview:                 report.MixedReview,
		ManualUnknown:               report.ManualUnknown,
		Empty:                       report.Empty,
		Indexed:                     report.Indexed,
		Issues:                      report.Issues,
	}
}

type parseDecisionEvidence struct {
	ID                   int64      `json:"id"`
	SourcePath           string     `json:"source_path"`
	SourceFilename       string     `json:"source_filename"`
	EventAt              time.Time  `json:"event_at"`
	MediaTypeGuessed     string     `json:"media_type_guessed,omitempty"`
	ParseMethod          string     `json:"parse_method,omitempty"`
	ParsedTitle          string     `json:"parsed_title,omitempty"`
	ParsedYear           *int       `json:"parsed_year,omitempty"`
	ParsedSeason         *int       `json:"parsed_season,omitempty"`
	ParsedEpisode        *int       `json:"parsed_episode,omitempty"`
	ParserStrippedTokens string     `json:"parser_stripped_tokens,omitempty"`
	TargetPath           string     `json:"target_path,omitempty"`
	TargetAt             *time.Time `json:"target_at,omitempty"`
	OrganizeOutcome      string     `json:"organize_outcome,omitempty"`
	OrganizeError        string     `json:"organize_error,omitempty"`
	JellyfinItemID       string     `json:"jellyfin_item_id,omitempty"`
	JellyfinIdentified   *bool      `json:"jellyfin_identified,omitempty"`
	AutoLabel            string     `json:"auto_label,omitempty"`
	MetadataState        string     `json:"metadata_state,omitempty"`
	MetadataError        string     `json:"metadata_error,omitempty"`
}

func parseDecisionEvidenceList(decisions []*database.ParseDecision) []parseDecisionEvidence {
	out := make([]parseDecisionEvidence, 0, len(decisions))
	for _, d := range decisions {
		if d == nil {
			continue
		}
		out = append(out, parseDecisionEvidence{
			ID:                   d.ID,
			SourcePath:           d.SourcePath,
			SourceFilename:       d.SourceFilename,
			EventAt:              d.EventAt,
			MediaTypeGuessed:     d.MediaTypeGuessed,
			ParseMethod:          d.ParseMethod,
			ParsedTitle:          d.ParsedTitle,
			ParsedYear:           d.ParsedYear,
			ParsedSeason:         d.ParsedSeason,
			ParsedEpisode:        d.ParsedEpisode,
			ParserStrippedTokens: d.ParserStrippedTokens,
			TargetPath:           d.TargetPath,
			TargetAt:             d.TargetAt,
			OrganizeOutcome:      d.OrganizeOutcome,
			OrganizeError:        d.OrganizeError,
			JellyfinItemID:       d.JellyfinItemID,
			JellyfinIdentified:   d.JellyfinIdentified,
			AutoLabel:            d.AutoLabel,
			MetadataState:        d.MetadataState,
			MetadataError:        d.MetadataError,
		})
	}
	return out
}

func (c Collector) housekeeping() housekeepingSnapshot {
	outstanding, err := c.DB.CountHousekeepingTasks()
	if err != nil {
		return housekeepingSnapshot{
			Counts:          map[string]int{},
			CreatedInWindow: map[string]int{},
			Outstanding:     map[string]int{},
			Error:           err.Error(),
		}
	}
	// ponytail: ceiling ~10k recent tasks; upgrade to COUNT(... WHERE created_at>=?) if backlog grows.
	recent, err := c.DB.ListHousekeepingTasks("", 10000)
	if err != nil {
		return housekeepingSnapshot{
			Counts:          outstanding,
			CreatedInWindow: map[string]int{},
			Outstanding:     outstanding,
			Error:           err.Error(),
		}
	}
	window := CountHousekeepingWindow(recent, c.Since, outstanding)
	recentOut := recent
	if len(recentOut) > 200 {
		recentOut = recentOut[:200]
	}
	return housekeepingSnapshot{
		Counts:          window.Outstanding,
		CreatedInWindow: window.CreatedInWindow,
		Outstanding:     window.Outstanding,
		Recent:          recentOut,
	}
}

func suspiciousFromDecisions(decisions []*database.ParseDecision) ([]SuspiciousItem, []SuspiciousItem) {
	suspicious := make([]SuspiciousItem, 0)
	pathFalsePositives := make([]SuspiciousItem, 0)
	type targetSource struct {
		name   string
		writes int
	}
	targetSources := make(map[string]map[string]targetSource)
	for _, d := range decisions {
		if d == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(d.OrganizeOutcome), "success") && strings.TrimSpace(d.TargetPath) != "" && strings.TrimSpace(d.SourcePath) != "" {
			sources := targetSources[d.TargetPath]
			if sources == nil {
				sources = make(map[string]targetSource)
				targetSources[d.TargetPath] = sources
			}
			source := sources[d.SourcePath]
			source.name = d.SourceFilename
			source.writes++
			sources[d.SourcePath] = source
		}
		name := strings.TrimSpace(d.ParsedTitle)
		if name == "" && d.TargetPath != "" {
			name = strings.TrimSuffix(filepath.Base(d.TargetPath), filepath.Ext(d.TargetPath))
		}
		if item := ClassifySuspiciousName(name, d.TargetPath); item.Category != "" {
			suspicious = append(suspicious, item)
		} else if d.TargetPath != "" {
			visibleName := strings.TrimSuffix(filepath.Base(d.TargetPath), filepath.Ext(d.TargetPath))
			if item := ClassifySuspiciousName(visibleName, d.TargetPath); item.Category != "" {
				suspicious = append(suspicious, item)
			}
		}
		if item := classifyParserDrift(d); item.Category != "" {
			suspicious = append(suspicious, item)
		}
		target, jellyfin, ok := parsePathMismatch(d.MetadataError)
		if ok {
			if item := ClassifyPathMismatch(target, jellyfin); item.Category != "" {
				pathFalsePositives = append(pathFalsePositives, item)
			} else {
				suspicious = append(suspicious, SuspiciousItem{
					Category: "path_mismatch",
					Name:     d.SourceFilename,
					Path:     d.TargetPath,
					Reason:   d.MetadataError,
				})
			}
		}
	}
	for target, sources := range targetSources {
		totalWrites := 0
		for _, source := range sources {
			totalWrites += source.writes
		}
		if totalWrites < 2 {
			continue
		}
		names := make([]string, 0, len(sources))
		for path, source := range sources {
			name := source.name
			if strings.TrimSpace(name) == "" {
				name = filepath.Base(path)
			}
			if source.writes > 1 {
				name = fmt.Sprintf("%s (%d writes)", name, source.writes)
			}
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > 4 {
			names = append(names[:4], fmt.Sprintf("and %d more", len(names)-4))
		}
		suspicious = append(suspicious, SuspiciousItem{
			Category: "target_collision",
			Path:     target,
			Reason:   "multiple successful parse decisions wrote to the same target: " + strings.Join(names, "; "),
		})
	}
	return suspicious, pathFalsePositives
}

func classifyParserDrift(d *database.ParseDecision) SuspiciousItem {
	if d == nil || d.ParsedTitle == "" || d.SourcePath == "" {
		return SuspiciousItem{}
	}
	switch strings.ToLower(strings.TrimSpace(d.ParseMethod)) {
	case "ai", "manual", "season_pack":
		return SuspiciousItem{}
	}

	switch strings.ToLower(strings.TrimSpace(d.MediaTypeGuessed)) {
	case "movie":
		info, err := naming.ParseMovieName(d.SourceFilename)
		if err != nil {
			info, err = naming.ParseMovieName(d.SourcePath)
		}
		if err != nil || info == nil {
			return SuspiciousItem{}
		}
		if sameNormalizedTitle(d.ParsedTitle, info.Title) && sameYear(d.ParsedYear, info.Year) {
			return SuspiciousItem{}
		}
		return SuspiciousItem{
			Category: "parser_drift",
			Name:     d.ParsedTitle,
			Path:     d.TargetPath,
			Reason:   fmt.Sprintf("current parser would produce movie title %q year %q", info.Title, info.Year),
		}
	case "tv":
		info, err := naming.ParseTVShowFromPath(d.SourcePath)
		if err != nil || info == nil {
			return SuspiciousItem{}
		}
		if sameNormalizedTitle(d.ParsedTitle, info.Title) &&
			sameIntPtr(d.ParsedSeason, info.Season) &&
			sameIntPtr(d.ParsedEpisode, info.Episode) {
			return SuspiciousItem{}
		}
		return SuspiciousItem{
			Category: "parser_drift",
			Name:     d.ParsedTitle,
			Path:     d.TargetPath,
			Reason:   fmt.Sprintf("current parser would produce TV identity %q S%02dE%02d", info.Title, info.Season, info.Episode),
		}
	default:
		return SuspiciousItem{}
	}
}

func sameNormalizedTitle(a, b string) bool {
	return normalizeDriftTitle(a) == normalizeDriftTitle(b)
}

func normalizeDriftTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func sameYear(stored *int, parsed string) bool {
	if stored == nil {
		return true
	}
	if parsed == "" {
		return false
	}
	return fmt.Sprintf("%d", *stored) == parsed
}

func sameIntPtr(stored *int, parsed int) bool {
	if stored == nil || parsed == 0 {
		return stored == nil && parsed == 0
	}
	return *stored == parsed
}

func parsePathMismatch(msg string) (target, jellyfin string, ok bool) {
	const prefix = `target path "`
	const mid = `" does not match jellyfin path "`
	if !strings.HasPrefix(msg, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(msg, prefix)
	idx := strings.Index(rest, mid)
	if idx < 0 {
		return "", "", false
	}
	target = rest[:idx]
	jellyfin = strings.TrimSuffix(rest[idx+len(mid):], `"`)
	return target, jellyfin, target != "" && jellyfin != ""
}

type mediaInventorySnapshot struct {
	TotalFiles          int            `json:"total_files"`
	ByType              map[string]int `json:"by_type"`
	DuplicateGroups     int            `json:"duplicate_groups"`
	DuplicateFiles      int            `json:"duplicate_files"`
	SpaceReclaimable    int64          `json:"space_reclaimable"`
	NonCompliantFiles   int            `json:"non_compliant_files"`
	QualityDistribution map[string]int `json:"quality_distribution"`
	Error               string         `json:"error,omitempty"`
}

func (c Collector) mediaInventory() mediaInventorySnapshot {
	snap := mediaInventorySnapshot{ByType: make(map[string]int), QualityDistribution: make(map[string]int)}
	stats, err := c.DB.GetConsolidationStats()
	if err != nil {
		snap.Error = err.Error()
		return snap
	}
	snap.TotalFiles = stats.TotalFiles
	snap.DuplicateGroups = stats.DuplicateGroups
	snap.DuplicateFiles = stats.DuplicateFiles
	snap.SpaceReclaimable = stats.SpaceReclaimable
	snap.NonCompliantFiles = stats.NonCompliantFiles

	for _, mt := range []string{"movie", "episode"} {
		n, err := c.DB.CountMediaFilesByType(mt)
		if err == nil {
			snap.ByType[mt] = n
		}
	}

	files, err := c.DB.GetAllMediaFiles()
	if err != nil {
		return snap
	}
	for _, f := range files {
		switch {
		case f.QualityScore >= 10:
			snap.QualityDistribution["10+"]++
		case f.QualityScore >= 7:
			snap.QualityDistribution["7-9"]++
		case f.QualityScore >= 4:
			snap.QualityDistribution["4-6"]++
		case f.QualityScore >= 1:
			snap.QualityDistribution["1-3"]++
		default:
			snap.QualityDistribution["0"]++
		}
	}
	return snap
}

func (c Collector) configSnapshot() map[string]any {
	cfg, err := config.Load()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{
		"watch_tv":       cfg.Watch.TV,
		"watch_movies":   cfg.Watch.Movies,
		"library_tv":     cfg.Libraries.TV,
		"library_movies": cfg.Libraries.Movies,
		"scan_frequency": cfg.Daemon.ScanFrequency,
		"ai_model":       cfg.AI.Model,
		"ai_enabled":     cfg.AI.Enabled,
		"jellyfin_url":   cfg.Jellyfin.URL,
		"sonarr_url":     cfg.Sonarr.URL,
		"radarr_url":     cfg.Radarr.URL,
	}
}

func (c Collector) daemonLogExcerpt() string {
	var failures []string
	type candidate struct {
		path string
		mod  time.Time
	}
	var candidates []candidate
	if c.LogDir != "" {
		entries, err := os.ReadDir(c.LogDir)
		if err != nil {
			failures = append(failures, fmt.Sprintf("log dir %s: %v", c.LogDir, err))
		} else {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if isNoisyWebLogName(name) {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					failures = append(failures, fmt.Sprintf("stat %s: %v", filepath.Join(c.LogDir, name), err))
					continue
				}
				candidates = append(candidates, candidate{
					path: filepath.Join(c.LogDir, name),
					mod:  info.ModTime(),
				})
			}
		}
	} else {
		failures = append(failures, "no configured file log path")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].mod.Before(candidates[j].mod)
	})

	var lines []string
	var webWarns, webErrs int
	scanWeb := func(data string) {
		w, e := countWebWarnings(data, c.Since)
		webWarns += w
		webErrs += e
	}
	for _, cand := range candidates {
		data, err := readLogFile(cand.path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("read %s: %v", cand.path, err))
			continue
		}
		if strings.TrimSpace(data) == "" {
			failures = append(failures, fmt.Sprintf("read %s: empty log file", cand.path))
			continue
		}
		scanWeb(data)
		lines = append(lines, filterDaemonLogLines(data, c.Since)...)
	}
	// Also scan noisy web/access siblings for warning totals only.
	if c.LogDir != "" {
		if entries, err := os.ReadDir(c.LogDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !isNoisyWebLogName(entry.Name()) {
					continue
				}
				data, err := readLogFile(filepath.Join(c.LogDir, entry.Name()))
				if err != nil {
					continue
				}
				scanWeb(data)
			}
		}
	}
	if len(lines) > 0 {
		text := strings.Join(lines, "\n")
		if webWarns > 0 || webErrs > 0 {
			text += fmt.Sprintf("\n\nweb warnings: warn=%d error=%d (excluded from daemon excerpt)", webWarns, webErrs)
		}
		return text
	}

	if text, err := journalctlExcerpt(c.Since); err == nil {
		return text
	} else {
		failures = append(failures, fmt.Sprintf("journalctl plex2jellyfin-daemon: %v", err))
	}

	return "daemon log unavailable\n" + strings.Join(failures, "\n")
}

func isNoisyWebLogName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "web") || strings.Contains(lower, "access")
}

func readLogFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		r = gz
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func filterDaemonLogLines(content string, since time.Time) []string {
	raw := strings.Split(content, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if line == "" {
			continue
		}
		if isWebAccessLogLine(line) {
			continue
		}
		if !since.IsZero() {
			if ts, ok := parseLogLineTimestamp(line); ok && ts.Before(since) {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

func isWebAccessLogLine(line string) bool {
	return strings.Contains(line, "] [web] ")
}

func countWebWarnings(content string, since time.Time) (warns, errs int) {
	for _, line := range strings.Split(content, "\n") {
		if line == "" || !isWebAccessLogLine(line) {
			continue
		}
		if !since.IsZero() {
			if ts, ok := parseLogLineTimestamp(line); ok && ts.Before(since) {
				continue
			}
		}
		switch {
		case strings.Contains(line, " [ERROR] "):
			errs++
		case strings.Contains(line, " [WARN] "):
			warns++
		}
	}
	return warns, errs
}

func parseLogLineTimestamp(line string) (time.Time, bool) {
	space := strings.IndexByte(line, ' ')
	if space < 1 {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, line[:space])
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return writeBytes(path, data)
}

func writeText(path, s string) error {
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return writeBytes(path, []byte(s))
}

func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func updateLatestLink(bundle BundlePaths) error {
	if err := os.RemoveAll(bundle.LatestLink); err != nil {
		return fmt.Errorf("remove latest link: %w", err)
	}
	if err := os.Symlink(bundle.Dir, bundle.LatestLink); err != nil {
		return fmt.Errorf("update latest link: %w", err)
	}
	return nil
}
