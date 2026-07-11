package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemonctl"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/llm"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/spf13/cobra"
)

// setupDeps injects every side effect the wizard performs so tests can
// script a full run without a network, daemon, or real home directory.
type setupDeps struct {
	loadConfig    func() (*config.Config, error)
	preflight     func(setupdomain.Draft) []setupdomain.FieldError
	testSonarr    func(url, apiKey string) error
	testRadarr    func(url, apiKey string) error
	testJellyfin  func(url, apiKey string) error
	checkSonarr   func(url, apiKey string) ([]service.HealthIssue, error)
	fixSonarr     func(url, apiKey string, issues []service.HealthIssue) error
	checkRadarr   func(url, apiKey string) ([]service.HealthIssue, error)
	fixRadarr     func(url, apiKey string, issues []service.HealthIssue) error
	listModels    func(endpoint string) ([]string, error)
	activate      func(ctx context.Context) error
	generateToken func() (string, error)
	runtime       setupdomain.RuntimeInfo
}

func defaultSetupDeps() setupDeps {
	return setupDeps{
		loadConfig: config.Load,
		preflight:  api.ValidateSetupPaths,
		testSonarr: func(url, apiKey string) error {
			_, err := sonarr.NewClient(sonarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}).GetSystemStatus()
			return err
		},
		testRadarr: func(url, apiKey string) error {
			_, err := radarr.NewClient(radarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}).GetSystemStatus()
			return err
		},
		testJellyfin: func(url, apiKey string) error {
			_, err := jellyfin.NewClient(jellyfin.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}).GetSystemInfo()
			return err
		},
		checkSonarr: func(url, apiKey string) ([]service.HealthIssue, error) {
			return service.CheckSonarrConfig(sonarr.NewClient(sonarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}))
		},
		fixSonarr: func(url, apiKey string, issues []service.HealthIssue) error {
			_, err := service.FixSonarrIssues(sonarr.NewClient(sonarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}), issues, false)
			return err
		},
		checkRadarr: func(url, apiKey string) ([]service.HealthIssue, error) {
			return service.CheckRadarrConfig(radarr.NewClient(radarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}))
		},
		fixRadarr: func(url, apiKey string, issues []service.HealthIssue) error {
			_, err := service.FixRadarrIssues(radarr.NewClient(radarr.Config{URL: url, APIKey: apiKey, Timeout: 5 * time.Second}), issues, false)
			return err
		},
		listModels: func(endpoint string) ([]string, error) {
			adapter := llm.NewOllamaAdapter("setup", "setup", endpoint, "")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			models, err := adapter.ListModels(ctx)
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(models))
			for _, m := range models {
				names = append(names, m.ID)
			}
			return names, nil
		},
		activate:      activateDaemonCLI,
		generateToken: newSetupSecret,
		runtime:       setupdomain.DetectRuntime(),
	}
}

// activateDaemonCLI mirrors the web wizard's activation: reload a running
// daemon over IPC, otherwise launch the daemon binary, then poll readiness.
func activateDaemonCLI(ctx context.Context) error {
	dir, err := paths.Plex2JellyfinDir()
	if err != nil {
		return err
	}
	client := ipc.NewClient(filepath.Join(dir, "control.sock"))

	if _, err := client.Call(ctx, ipc.CmdStatus, nil); err == nil {
		raw, err := client.Call(ctx, ipc.CmdReload, nil)
		if err != nil {
			return fmt.Errorf("reload daemon: %w", err)
		}
		var result struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(raw, &result); err == nil && !result.OK {
			return errors.New("daemon rejected the setup configuration")
		}
	} else {
		binary := "plex2jellyfin-daemon"
		if self, err := os.Executable(); err == nil {
			binary = filepath.Join(filepath.Dir(self), "plex2jellyfin-daemon")
		}
		if err := daemonctl.New(binary, filepath.Join(dir, "plex2jellyfin-daemon.log")).Start(); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
	}

	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := client.Call(deadline, ipc.CmdStatus, nil); err == nil {
			return nil
		}
		select {
		case <-deadline.Done():
			return errors.New("daemon did not become ready within 10s")
		case <-ticker.C:
		}
	}
}

func newSetupSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// prompter is a line-based question helper over the wizard's stdio.
type prompter struct {
	in  *bufio.Scanner
	out io.Writer
}

func (p *prompter) ask(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	if !p.in.Scan() {
		if err := p.in.Err(); err != nil {
			return "", err
		}
		return "", errors.New("input ended before setup finished")
	}
	answer := strings.TrimSpace(p.in.Text())
	if answer == "" {
		return def, nil
	}
	return answer, nil
}

func (p *prompter) askBool(label string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	answer, err := p.ask(label+" ("+hint+")", "")
	if err != nil {
		return false, err
	}
	if answer == "" {
		return def, nil
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		fmt.Fprintln(p.out, "  please answer y or n")
		return p.askBool(label, def)
	}
}

// askPaths collects a comma-separated list of directories.
func (p *prompter) askPaths(label string, def []string) ([]string, error) {
	answer, err := p.ask(label+" (comma-separated, empty to skip)", strings.Join(def, ", "))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(answer) == "" {
		return nil, nil
	}
	parts := strings.Split(answer, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

func newSetupCmd() *cobra.Command {
	return newSetupCmdWithDeps(defaultSetupDeps(), os.Stdin, os.Stdout)
}

func newSetupCmdWithDeps(deps setupDeps, stdin io.Reader, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive first-run configuration wizard",
		Long: `Configure plex2jellyfin from the terminal: watch and library paths,
optional Sonarr/Radarr/Jellyfin connections, optional Ollama matching, and
runtime behavior. Validates every path, saves the config atomically, and
activates the daemon — the same flow as the web UI's /setup wizard.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupWizard(cmd.Context(), deps, stdin, stdout)
		},
	}
}

func runSetupWizard(ctx context.Context, deps setupDeps, stdin io.Reader, stdout io.Writer) error {
	p := &prompter{in: bufio.NewScanner(stdin), out: stdout}

	current, err := deps.loadConfig()
	if err != nil {
		current = config.DefaultConfig()
	}
	if !setupdomain.NeedsSetup(current) {
		rerun, err := p.askBool("This install is already configured. Re-run setup anyway?", false)
		if err != nil {
			return err
		}
		if !rerun {
			fmt.Fprintln(stdout, "Nothing to do.")
			return nil
		}
	}

	draft := setupdomain.DraftFromConfig(current)

	fmt.Fprintln(stdout, "\n— Media paths —")
	fmt.Fprintln(stdout, "Configure TV, Movies, or both. Each configured type needs an")
	fmt.Fprintln(stdout, "incoming (download) path and a library path.")
	if draft.Watch.TV, err = p.askPaths("TV incoming paths", draft.Watch.TV); err != nil {
		return err
	}
	if draft.Libraries.TV, err = p.askPaths("TV library paths", draft.Libraries.TV); err != nil {
		return err
	}
	if draft.Watch.Movies, err = p.askPaths("Movie incoming paths", draft.Watch.Movies); err != nil {
		return err
	}
	if draft.Libraries.Movies, err = p.askPaths("Movie library paths", draft.Libraries.Movies); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "\n— Connected services (optional) —")
	if err := promptService(p, "Sonarr", "http://localhost:8989", &draft.Sonarr, deps.testSonarr); err != nil {
		return err
	}
	if draft.Sonarr.Enabled {
		if err := promptArrCompatibility(p, stdout, "Sonarr", draft.Sonarr, deps.checkSonarr, deps.fixSonarr); err != nil {
			return err
		}
	}
	if err := promptService(p, "Radarr", "http://localhost:7878", &draft.Radarr, deps.testRadarr); err != nil {
		return err
	}
	if draft.Radarr.Enabled {
		if err := promptArrCompatibility(p, stdout, "Radarr", draft.Radarr, deps.checkRadarr, deps.fixRadarr); err != nil {
			return err
		}
	}

	jellyfinDraft := setupdomain.ServiceDraft{Enabled: draft.Jellyfin.Enabled, URL: draft.Jellyfin.URL, APIKey: draft.Jellyfin.APIKey}
	if err := promptService(p, "Jellyfin", "http://localhost:8096", &jellyfinDraft, deps.testJellyfin); err != nil {
		return err
	}
	draft.Jellyfin.Enabled = jellyfinDraft.Enabled
	draft.Jellyfin.URL = jellyfinDraft.URL
	draft.Jellyfin.APIKey = jellyfinDraft.APIKey

	fmt.Fprintln(stdout, "\n— AI matching (optional) —")
	if draft.AI.Enabled, err = p.askBool("Use a local Ollama instance for ambiguous filenames?", draft.AI.Enabled); err != nil {
		return err
	}
	if draft.AI.Enabled {
		if draft.AI.Endpoint, err = p.ask("Ollama endpoint", firstNonEmpty(draft.AI.Endpoint, "http://localhost:11434")); err != nil {
			return err
		}
		models, err := deps.listModels(draft.AI.Endpoint)
		if err != nil {
			fmt.Fprintf(stdout, "  could not list models: %v\n", err)
		} else if len(models) > 0 {
			fmt.Fprintf(stdout, "  installed models: %s\n", strings.Join(models, ", "))
		}
		if draft.AI.PrimaryModel, err = p.ask("Primary model", draft.AI.PrimaryModel); err != nil {
			return err
		}
		if draft.AI.FallbackModel, err = p.ask("Fallback model (empty for none)", draft.AI.FallbackModel); err != nil {
			return err
		}
	}

	fmt.Fprintln(stdout, "\n— Runtime behavior —")
	if draft.Runtime.ScanFrequency, err = p.ask("Library scan frequency", firstNonEmpty(draft.Runtime.ScanFrequency, "5m")); err != nil {
		return err
	}
	if draft.Runtime.DeleteSource, err = p.askBool("Move files (delete the source after import)?", draft.Runtime.DeleteSource); err != nil {
		return err
	}
	if draft.Runtime.VerifyChecksums, err = p.askBool("Verify checksums after transfers?", draft.Runtime.VerifyChecksums); err != nil {
		return err
	}
	if deps.runtime.Kind == setupdomain.RuntimeContainer {
		fmt.Fprintf(stdout, "Container runtime detected (uid %d, gid %d): ownership is controlled by PUID/PGID.\n", deps.runtime.UID, deps.runtime.GID)
		draft.Runtime.Permissions = config.PermissionsConfig{}
	} else {
		if draft.Runtime.Permissions.User, err = p.ask("Chown moved files to user (empty to skip)", draft.Runtime.Permissions.User); err != nil {
			return err
		}
		if draft.Runtime.Permissions.User != "" {
			if draft.Runtime.Permissions.Group, err = p.ask("Group", firstNonEmpty(draft.Runtime.Permissions.Group, draft.Runtime.Permissions.User)); err != nil {
				return err
			}
			if draft.Runtime.Permissions.FileMode, err = p.ask("File mode", firstNonEmpty(draft.Runtime.Permissions.FileMode, "0644")); err != nil {
				return err
			}
			if draft.Runtime.Permissions.DirMode, err = p.ask("Directory mode", firstNonEmpty(draft.Runtime.Permissions.DirMode, "0755")); err != nil {
				return err
			}
		}
	}

	// Validate the draft and preflight every path before writing anything.
	if errs := setupdomain.ValidateDraft(draft, deps.runtime); len(errs) > 0 {
		printFieldErrors(stdout, errs)
		return errors.New("setup draft is invalid")
	}
	if errs := deps.preflight(draft); len(errs) > 0 {
		printFieldErrors(stdout, errs)
		return errors.New("path preflight failed")
	}

	fmt.Fprintln(stdout, "\n— Review —")
	printDraftSummary(stdout, draft)
	confirmed, err := p.askBool("Write this configuration and activate the daemon?", true)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(stdout, "Aborted; nothing was written.")
		return nil
	}

	// Same transaction as the web wizard: save with completed=false, activate
	// the daemon, then mark completed. A failed activation leaves the marker
	// incomplete so the next run returns to setup.
	fresh, err := deps.loadConfig()
	if err != nil {
		fresh = config.DefaultConfig()
	}
	candidate := setupdomain.ApplyDraft(fresh, draft)
	if candidate.Jellyfin.Enabled && candidate.Jellyfin.WebhookSecret == "" {
		secret, err := deps.generateToken()
		if err != nil {
			return fmt.Errorf("generate webhook secret: %w", err)
		}
		candidate.Jellyfin.WebhookSecret = secret
		candidate.Jellyfin.PluginSharedSecret = secret
	}
	if err := candidate.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintln(stdout, "Activating daemon…")
	if err := deps.activate(ctx); err != nil {
		fmt.Fprintf(stdout, "Daemon activation failed: %v\n", err)
		fmt.Fprintln(stdout, "The configuration is saved; re-run 'plex2jellyfin setup' or start the daemon service and it will pick it up.")
		return errors.New("daemon activation failed")
	}

	candidate.Setup.Completed = true
	if err := candidate.Save(); err != nil {
		return fmt.Errorf("mark setup complete: %w", err)
	}

	fmt.Fprintln(stdout, "\nSetup complete. The daemon is running.")
	fmt.Fprintln(stdout, "Next: open the web UI on :5522, or run 'plex2jellyfin scan' to index an existing library.")
	return nil
}

func promptService(p *prompter, name, defaultURL string, svc *setupdomain.ServiceDraft, test func(url, apiKey string) error) error {
	enabled, err := p.askBool("Connect "+name+"?", svc.Enabled)
	if err != nil {
		return err
	}
	svc.Enabled = enabled
	if !enabled {
		return nil
	}
	if svc.URL, err = p.ask(name+" URL", firstNonEmpty(svc.URL, defaultURL)); err != nil {
		return err
	}
	if svc.APIKey, err = p.ask(name+" API key", svc.APIKey); err != nil {
		return err
	}
	if err := test(svc.URL, svc.APIKey); err != nil {
		fmt.Fprintf(p.out, "  connection failed: %v\n", err)
		retry, rerr := p.askBool("Edit "+name+" settings and retry?", true)
		if rerr != nil {
			return rerr
		}
		if retry {
			return promptService(p, name, defaultURL, svc, test)
		}
		fmt.Fprintf(p.out, "  keeping %s settings without a passing test\n", name)
		return nil
	}
	fmt.Fprintf(p.out, "  %s connection OK\n", name)
	return nil
}

// promptArrCompatibility reports incompatible arr settings and only changes
// them after an explicit confirmation.
func promptArrCompatibility(p *prompter, out io.Writer, name string, svc setupdomain.ServiceDraft,
	check func(url, apiKey string) ([]service.HealthIssue, error),
	fix func(url, apiKey string, issues []service.HealthIssue) error) error {
	issues, err := check(svc.URL, svc.APIKey)
	if err != nil {
		fmt.Fprintf(out, "  %s compatibility check failed: %v\n", name, err)
		return nil
	}
	if len(issues) == 0 {
		fmt.Fprintf(out, "  %s settings are compatible\n", name)
		return nil
	}
	fmt.Fprintf(out, "  %s has %d incompatible setting(s):\n", name, len(issues))
	for _, issue := range issues {
		fmt.Fprintf(out, "    %s: %s (currently %s, needs %s)\n", issue.Severity, issue.Setting, issue.Current, issue.Expected)
	}
	apply, err := p.askBool("Apply these "+name+" fixes now?", false)
	if err != nil {
		return err
	}
	if !apply {
		fmt.Fprintf(out, "  leaving %s unchanged; run 'plex2jellyfin health --fix' later\n", name)
		return nil
	}
	if err := fix(svc.URL, svc.APIKey, issues); err != nil {
		fmt.Fprintf(out, "  fixing %s failed: %v\n", name, err)
		return nil
	}
	fmt.Fprintf(out, "  %s updated\n", name)
	return nil
}

func printFieldErrors(out io.Writer, errs []setupdomain.FieldError) {
	fmt.Fprintln(out, "\nSetup cannot continue:")
	for _, e := range errs {
		fmt.Fprintf(out, "  %s: %s\n", e.Field, e.Message)
	}
}

func printDraftSummary(out io.Writer, d setupdomain.Draft) {
	line := func(label, value string) {
		if value != "" {
			fmt.Fprintf(out, "  %-18s %s\n", label, value)
		}
	}
	line("TV incoming", strings.Join(d.Watch.TV, ", "))
	line("TV library", strings.Join(d.Libraries.TV, ", "))
	line("Movies incoming", strings.Join(d.Watch.Movies, ", "))
	line("Movies library", strings.Join(d.Libraries.Movies, ", "))
	svc := func(name string, s setupdomain.ServiceDraft) {
		if s.Enabled {
			line(name, s.URL)
		}
	}
	svc("Sonarr", d.Sonarr)
	svc("Radarr", d.Radarr)
	if d.Jellyfin.Enabled {
		line("Jellyfin", d.Jellyfin.URL)
	}
	if d.AI.Enabled {
		line("Ollama", d.AI.Endpoint+" ("+d.AI.PrimaryModel+")")
	}
	line("Scan frequency", d.Runtime.ScanFrequency)
	if d.Runtime.DeleteSource {
		line("Transfer mode", "move (delete source)")
	} else {
		line("Transfer mode", "copy")
	}
	if d.Runtime.Permissions.User != "" {
		line("Ownership", d.Runtime.Permissions.User+":"+d.Runtime.Permissions.Group)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
