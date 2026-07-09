package jellyfin

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type UnknownSeasonClass string

const (
	UnknownSeasonEmpty             UnknownSeasonClass = "empty"
	UnknownSeasonIndexed           UnknownSeasonClass = "indexed"
	UnknownSeasonRefreshRepairable UnknownSeasonClass = "refresh_repairable"
	UnknownSeasonFolderContext     UnknownSeasonClass = "folder_context"
	UnknownSeasonMixed             UnknownSeasonClass = "mixed_review"
	UnknownSeasonManual            UnknownSeasonClass = "manual_unknown"
)

var (
	sxxeyyPathRE            = regexp.MustCompile(`(?i)s[0-9]{1,2}[ ._-]*e[0-9]{1,3}`)
	seasonFolderPathRE      = regexp.MustCompile(`(?i)(^|[/\\])(season|series)[ ._-]*[0-9]{1,2}($|[/\\])`)
	randomishBasenamePathRE = regexp.MustCompile(`(?i)^[a-f0-9]{16,}$|^[a-z0-9]{24,}$`)
)

type User struct {
	ID     string      `json:"Id"`
	Name   string      `json:"Name"`
	Policy *UserPolicy `json:"Policy,omitempty"`
}

type UserPolicy struct {
	IsAdministrator bool `json:"IsAdministrator"`
}

type UnknownSeasonIssue struct {
	SeasonID               string             `json:"season_id"`
	SeriesID               string             `json:"series_id,omitempty"`
	SeriesName             string             `json:"series_name,omitempty"`
	DateCreated            string             `json:"date_created,omitempty"`
	Class                  UnknownSeasonClass `json:"class"`
	EpisodeCount           int                `json:"episode_count"`
	NullNumberCount        int                `json:"null_number_count"`
	SeasonPathCount        int                `json:"season_path_count"`
	SxxEyyPathCount        int                `json:"sxxeyy_path_count"`
	RandomishBasenameCount int                `json:"randomish_basename_count"`
	ExampleEpisodePath     string             `json:"example_episode_path,omitempty"`
	ExampleEpisodeName     string             `json:"example_episode_name,omitempty"`
	ExampleEpisodeID       string             `json:"example_episode_id,omitempty"`
	RefreshRepairable      bool               `json:"refresh_repairable"`
	RefreshTargetSeries    string             `json:"refresh_target_series,omitempty"`
}

type UnknownSeasonReport struct {
	UserID                    string               `json:"user_id"`
	Total                     int                  `json:"total"`
	Empty                     int                  `json:"empty"`
	Indexed                   int                  `json:"indexed"`
	RefreshRepairableSeasons  int                  `json:"refresh_repairable_seasons"`
	RefreshRepairableEpisodes int                  `json:"refresh_repairable_episodes"`
	RefreshCandidateSeasons   int                  `json:"refresh_candidate_seasons"`
	RefreshCandidateEpisodes  int                  `json:"refresh_candidate_episodes"`
	RandomishBasenameEpisodes int                  `json:"randomish_basename_episodes"`
	FolderContext             int                  `json:"folder_context"`
	MixedReview               int                  `json:"mixed_review"`
	ManualUnknown             int                  `json:"manual_unknown"`
	Issues                    []UnknownSeasonIssue `json:"issues"`
}

type UnknownSeasonRepairReport struct {
	Audit     UnknownSeasonReport   `json:"audit"`
	DryRun    bool                  `json:"dry_run"`
	Refreshed int                   `json:"refreshed"`
	Skipped   int                   `json:"skipped"`
	Errors    int                   `json:"errors"`
	Actions   []UnknownSeasonAction `json:"actions,omitempty"`
}

type UnknownSeasonAction struct {
	SeriesID   string `json:"series_id"`
	SeriesName string `json:"series_name,omitempty"`
	Action     string `json:"action"`
	Error      string `json:"error,omitempty"`
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	if err := c.getCtx(ctx, "/Users", &users); err != nil {
		return nil, fmt.Errorf("listing jellyfin users: %w", err)
	}
	return users, nil
}

func (c *Client) DefaultUserID(ctx context.Context) (string, error) {
	users, err := c.ListUsers(ctx)
	if err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no jellyfin users returned")
	}
	for _, user := range users {
		if user.Policy != nil && user.Policy.IsAdministrator && strings.TrimSpace(user.ID) != "" {
			return user.ID, nil
		}
	}
	for _, user := range users {
		if strings.TrimSpace(user.ID) != "" {
			return user.ID, nil
		}
	}
	return "", fmt.Errorf("no jellyfin user id returned")
}

func (c *Client) AuditUnknownSeasons(ctx context.Context, userID string) (*UnknownSeasonReport, error) {
	if strings.TrimSpace(userID) == "" {
		id, err := c.DefaultUserID(ctx)
		if err != nil {
			return nil, err
		}
		userID = id
	}

	seasons, err := c.listUnknownSeasons(ctx, userID)
	if err != nil {
		return nil, err
	}

	report := &UnknownSeasonReport{UserID: userID, Issues: make([]UnknownSeasonIssue, 0, len(seasons))}
	for _, season := range seasons {
		episodes, err := c.listSeasonEpisodes(ctx, userID, season.ID)
		if err != nil {
			return nil, err
		}
		issue := ClassifyUnknownSeasonEpisodes(episodes)
		issue.SeasonID = season.ID
		issue.SeriesID = season.SeriesID
		issue.SeriesName = firstNonEmpty(season.SeriesName, season.Name)
		issue.DateCreated = season.DateCreated
		if issue.RefreshRepairable || issue.SxxEyyPathCount > 0 {
			issue.RefreshTargetSeries = season.SeriesID
		}
		report.add(issue)
	}

	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Class != report.Issues[j].Class {
			return unknownSeasonClassRank(report.Issues[i].Class) < unknownSeasonClassRank(report.Issues[j].Class)
		}
		return report.Issues[i].SeriesName < report.Issues[j].SeriesName
	})
	return report, nil
}

func (c *Client) RepairUnknownSeasons(ctx context.Context, userID string, maxRefresh int, dryRun bool) (*UnknownSeasonRepairReport, error) {
	audit, err := c.AuditUnknownSeasons(ctx, userID)
	if err != nil {
		return nil, err
	}
	report := &UnknownSeasonRepairReport{Audit: *audit, DryRun: dryRun}
	seen := map[string]struct{}{}

	for _, issue := range audit.Issues {
		if strings.TrimSpace(issue.RefreshTargetSeries) == "" {
			report.Skipped++
			continue
		}
		if _, ok := seen[issue.RefreshTargetSeries]; ok {
			report.Skipped++
			continue
		}
		if maxRefresh > 0 && report.Refreshed >= maxRefresh {
			report.Skipped++
			continue
		}
		seen[issue.RefreshTargetSeries] = struct{}{}

		action := UnknownSeasonAction{SeriesID: issue.RefreshTargetSeries, SeriesName: issue.SeriesName}
		if dryRun {
			action.Action = "would_refresh"
			report.Refreshed++
			report.Actions = append(report.Actions, action)
			continue
		}
		if err := c.RefreshItemMetadataRecursive(issue.RefreshTargetSeries); err != nil {
			action.Action = "failed"
			action.Error = err.Error()
			report.Errors++
		} else {
			action.Action = "refreshed"
			report.Refreshed++
		}
		report.Actions = append(report.Actions, action)
	}

	return report, nil
}

func ClassifyUnknownSeasonEpisodes(episodes []Item) UnknownSeasonIssue {
	issue := UnknownSeasonIssue{EpisodeCount: len(episodes)}
	if len(episodes) == 0 {
		issue.Class = UnknownSeasonEmpty
		return issue
	}

	for _, episode := range episodes {
		if episode.IndexNumber == nil || episode.ParentIndexNumber == nil {
			issue.NullNumberCount++
		}
		if pathHasSeasonFolder(episode.Path) {
			issue.SeasonPathCount++
		}
		if pathHasSxxEyy(episode.Path) {
			issue.SxxEyyPathCount++
		}
		if pathHasRandomishBasename(episode.Path) {
			issue.RandomishBasenameCount++
		}
		if issue.ExampleEpisodePath == "" {
			issue.ExampleEpisodePath = episode.Path
			issue.ExampleEpisodeName = episode.Name
			issue.ExampleEpisodeID = episode.ID
		}
	}

	switch {
	case issue.NullNumberCount == 0:
		issue.Class = UnknownSeasonIndexed
	case issue.SxxEyyPathCount == issue.EpisodeCount:
		issue.Class = UnknownSeasonRefreshRepairable
		issue.RefreshRepairable = true
	case issue.SxxEyyPathCount > 0:
		issue.Class = UnknownSeasonMixed
	case issue.SeasonPathCount == issue.EpisodeCount:
		issue.Class = UnknownSeasonFolderContext
	case issue.SeasonPathCount > 0 || issue.SxxEyyPathCount > 0:
		issue.Class = UnknownSeasonMixed
	default:
		issue.Class = UnknownSeasonManual
	}
	return issue
}

func (r *UnknownSeasonReport) add(issue UnknownSeasonIssue) {
	r.Total++
	r.Issues = append(r.Issues, issue)
	switch issue.Class {
	case UnknownSeasonEmpty:
		r.Empty++
	case UnknownSeasonIndexed:
		r.Indexed++
	case UnknownSeasonRefreshRepairable:
		r.RefreshRepairableSeasons++
		r.RefreshRepairableEpisodes += issue.EpisodeCount
	case UnknownSeasonFolderContext:
		r.FolderContext++
	case UnknownSeasonMixed:
		r.MixedReview++
	case UnknownSeasonManual:
		r.ManualUnknown++
	}
	if strings.TrimSpace(issue.RefreshTargetSeries) != "" {
		r.RefreshCandidateSeasons++
		r.RefreshCandidateEpisodes += issue.SxxEyyPathCount
	}
	r.RandomishBasenameEpisodes += issue.RandomishBasenameCount
}

func (c *Client) listUnknownSeasons(ctx context.Context, userID string) ([]Item, error) {
	query := url.Values{}
	query.Set("IncludeItemTypes", "Season")
	query.Set("Recursive", "true")
	query.Set("SearchTerm", "Season Unknown")
	query.Set("Fields", "Path,DateCreated,ParentId,SeriesName,SeriesId")
	query.Set("Limit", "10000")

	var resp ItemsResponse
	if err := c.getCtx(ctx, "/Users/"+url.PathEscape(userID)+"/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("listing unknown seasons: %w", err)
	}
	out := make([]Item, 0, len(resp.Items))
	for _, item := range resp.Items {
		if strings.EqualFold(strings.TrimSpace(item.Name), "Season Unknown") {
			out = append(out, item)
		}
	}
	return out, nil
}

func (c *Client) listSeasonEpisodes(ctx context.Context, userID, seasonID string) ([]Item, error) {
	query := url.Values{}
	query.Set("ParentId", seasonID)
	query.Set("IncludeItemTypes", "Episode")
	query.Set("Recursive", "true")
	query.Set("Fields", "Path,IndexNumber,ParentIndexNumber,SeriesId,SeriesName")
	query.Set("Limit", strconv.Itoa(1000))

	var resp ItemsResponse
	if err := c.getCtx(ctx, "/Users/"+url.PathEscape(userID)+"/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("listing season episodes %s: %w", seasonID, err)
	}
	return resp.Items, nil
}

func pathHasSxxEyy(path string) bool {
	return sxxeyyPathRE.MatchString(path)
}

func pathHasSeasonFolder(path string) bool {
	return seasonFolderPathRE.MatchString(path)
}

func pathHasRandomishBasename(path string) bool {
	base := strings.ReplaceAll(filepath.Base(path), "\\", "/")
	stem := strings.TrimSuffix(filepath.Base(base), filepath.Ext(base))
	return randomishBasenamePathRE.MatchString(stem)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func unknownSeasonClassRank(class UnknownSeasonClass) int {
	switch class {
	case UnknownSeasonRefreshRepairable:
		return 0
	case UnknownSeasonFolderContext:
		return 1
	case UnknownSeasonMixed:
		return 2
	case UnknownSeasonManual:
		return 3
	case UnknownSeasonEmpty:
		return 4
	case UnknownSeasonIndexed:
		return 5
	default:
		return 9
	}
}
