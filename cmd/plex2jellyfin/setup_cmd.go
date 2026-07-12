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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/clitheme"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemonctl"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	"github.com/Nomadcxx/plex2jellyfin/internal/llm"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/privilege"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/chzyer/readline"
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
	activateWeb   func() error
	webUnitOK     func() bool
	initialScan   func(ctx context.Context, out io.Writer, draft setupdomain.Draft) error
	generateToken func() (string, error)
	runtime       setupdomain.RuntimeInfo
	pluginEngine  func(url, apiKey string) pluginEngine
	advertiseIP   func() string
	saveConfig    func(*config.Config) error
	permDefaults  func() config.PermissionsConfig
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
		activate:    activateDaemonCLI,
		activateWeb: activateWebCLI,
		webUnitOK:   webUnitInstalled,
		initialScan: func(ctx context.Context, out io.Writer, draft setupdomain.Draft) error {
			return runSetupInitialScan(ctx, out, draft)
		},
		generateToken: newSetupSecret,
		runtime:       setupdomain.DetectRuntime(),
		pluginEngine: func(url, apiKey string) pluginEngine {
			return plugininstall.New(url, apiKey, &http.Client{Timeout: 15 * time.Second})
		},
		advertiseIP:  setupdomain.DetectAdvertiseIP,
		saveConfig:   func(c *config.Config) error { return c.Save() },
		permDefaults: setupdomain.DefaultPermissions,
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

func webUnitInstalled() bool {
	return exec.Command("systemctl", "cat", "plex2jellyfin-web.service").Run() == nil
}

// activateWebCLI enables and starts the system web unit (same as the TUI
// installer). Uses sudo when not already root — systemctl needs privileges.
func activateWebCLI() error {
	args := []string{"systemctl", "enable", "--now", "plex2jellyfin-web.service"}
	var cmd *exec.Cmd
	if privilege.NeedsRoot() {
		cmd = exec.Command("sudo", args...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func printWebServiceSkipHints(stdout io.Writer) {
	clitheme.Muted(stdout, "Web UI left stopped. When you want it:")
	clitheme.Muted(stdout, "  sudo systemctl enable --now plex2jellyfin-web")
	clitheme.Muted(stdout, "Check services: systemctl status plex2jellyfin-daemon plex2jellyfin-web")
}

func newSetupSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// prompter is a line-based question helper over the wizard's stdio.
// On a real TTY it uses readline so arrow keys edit the line instead of
// inserting raw escape sequences (^[[A etc.).
type prompter struct {
	in    *bufio.Scanner
	out   io.Writer
	rl    *readline.Instance
	useRL bool
}

func newPrompter(stdin io.Reader, stdout io.Writer) *prompter {
	p := &prompter{in: bufio.NewScanner(stdin), out: stdout}
	f, ok := stdin.(*os.File)
	if !ok || !readline.IsTerminal(int(f.Fd())) {
		return p
	}
	outFile, _ := stdout.(*os.File)
	if outFile == nil {
		outFile = os.Stdout
	}
	rl, err := readline.NewEx(&readline.Config{
		Stdin:           f,
		Stdout:          outFile,
		HistoryLimit:    -1,
		HistoryFile:     "",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return p
	}
	p.rl = rl
	p.useRL = true
	return p
}

func (p *prompter) Close() {
	if p.rl != nil {
		_ = p.rl.Close()
		p.rl = nil
		p.useRL = false
	}
}

func (p *prompter) ask(label, def string) (string, error) {
	if p.useRL {
		// Prefill the editable buffer with the default so arrow keys can revise
		// long values (paths, etc.). Prompt stays a simple label.
		return p.askReadline(label+": ", def)
	}
	prompt := label + ": "
	if def != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, def)
	}
	fmt.Fprint(p.out, prompt)
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

func (p *prompter) askReadline(prompt, def string) (string, error) {
	p.rl.SetPrompt(prompt)
	if def != "" {
		p.rl.Operation.SetBuffer(def)
	} else {
		p.rl.Operation.SetBuffer("")
	}
	line, err := p.rl.Readline()
	if err != nil {
		if errors.Is(err, readline.ErrInterrupt) {
			return "", errors.New("setup interrupted")
		}
		if errors.Is(err, io.EOF) {
			return "", errors.New("input ended before setup finished")
		}
		return "", err
	}
	answer := strings.TrimSpace(line)
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

// askModelChoice prints a numbered model list and accepts an index or name.
// When allowEmpty is true, a blank answer returns "" (used for optional fallback).
func (p *prompter) askModelChoice(label string, options []string, def string, allowEmpty bool) (string, error) {
	if len(options) == 0 {
		return p.ask(label, def)
	}
	for i, opt := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, opt)
	}
	hint := modelChoiceDefaultHint(options, def, allowEmpty)
	prompt := label
	if allowEmpty {
		prompt = label + " (empty for none)"
	}
	for {
		answer, err := p.ask(prompt, hint)
		if err != nil {
			return "", err
		}
		if allowEmpty && answer == "" {
			return "", nil
		}
		got, err := resolveModelChoice(answer, options, hint)
		if err != nil {
			fmt.Fprintf(p.out, "  %v\n", err)
			continue
		}
		return got, nil
	}
}

func modelChoiceDefaultHint(options []string, def string, allowEmpty bool) string {
	if def == "" {
		if allowEmpty {
			return ""
		}
		if len(options) > 0 {
			return "1"
		}
		return ""
	}
	for i, opt := range options {
		if opt == def {
			return fmt.Sprintf("%d", i+1)
		}
	}
	return def
}

// resolveModelChoice maps a typed answer to a model name.
// Empty answer uses def (which may itself be a 1-based index string).
func resolveModelChoice(answer string, options []string, def string) (string, error) {
	raw := strings.TrimSpace(answer)
	if raw == "" {
		raw = strings.TrimSpace(def)
	}
	if raw == "" {
		return "", errors.New("pick a model number or name")
	}
	if n, err := strconv.Atoi(raw); err == nil {
		if n < 1 || n > len(options) {
			return "", fmt.Errorf("choose a number between 1 and %d", len(options))
		}
		return options[n-1], nil
	}
	return raw, nil
}

func (p *prompter) askScanFrequency(def string) (string, error) {
	for {
		raw, err := p.ask("Library scan frequency (e.g. 5m, 10m, 1h — bare 10 means 10m)", def)
		if err != nil {
			return "", err
		}
		normalized, err := setupdomain.NormalizeScanFrequency(raw)
		if err != nil {
			fmt.Fprintf(p.out, "  invalid frequency %q — use a duration like 5m or 10m\n", raw)
			continue
		}
		return normalized, nil
	}
}

func (p *prompter) askOctalMode(label, def string) (string, error) {
	for {
		raw, err := p.ask(label+" (octal, e.g. "+def+")", def)
		if err != nil {
			return "", err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			raw = def
		}
		if _, err := strconv.ParseUint(raw, 8, 32); err != nil {
			fmt.Fprintf(p.out, "  %q is not an octal mode — try %s\n", raw, def)
			continue
		}
		return raw, nil
	}
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
			// TUI installer parity: escalate once at the start so systemd,
			// the initial scan, and post-scan chown all run as root with
			// SUDO_USER preserved for path/ownership resolution.
			if privilege.NeedsRoot() {
				return privilege.Escalate("configure services, scan libraries, and set file ownership")
			}
			return runSetupWizard(cmd.Context(), deps, stdin, stdout)
		},
	}
}

func runSetupWizard(ctx context.Context, deps setupDeps, stdin io.Reader, stdout io.Writer) error {
	p := newPrompter(stdin, stdout)
	defer p.Close()

	clitheme.PrintBanner(stdout, version)
	clitheme.Muted(stdout, "Interactive first-run setup: watch and library paths, optional Sonarr/Radarr/Jellyfin/AI,")
	clitheme.Muted(stdout, "then save config and activate the daemon. Empty answers keep defaults; Ctrl+C aborts.")

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

	clitheme.Section(stdout, "Media paths")
	clitheme.Muted(stdout, "Configure TV, Movies, or both. Each configured type needs an")
	clitheme.Muted(stdout, "incoming (download) path and a library path.")
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

	clitheme.Section(stdout, "Connected services (optional)")
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

	pluginState := wizardPluginStep(ctx, p, stdout, deps, jellyfinDraft)

	clitheme.Section(stdout, "AI matching (optional)")
	clitheme.Muted(stdout, "Most filenames parse without AI. Enable Ollama only for edge cases:")
	clitheme.Muted(stdout, "low-confidence titles, missing years, or release tags the regex parser cannot fix.")
	if draft.AI.Enabled, err = p.askBool("Use a local Ollama instance for ambiguous filenames?", draft.AI.Enabled); err != nil {
		return err
	}
	if draft.AI.Enabled {
		if draft.AI.Endpoint, err = p.ask("Ollama endpoint", firstNonEmpty(draft.AI.Endpoint, "http://localhost:11434")); err != nil {
			return err
		}
		models, err := deps.listModels(draft.AI.Endpoint)
		if err != nil {
			clitheme.Warn(stdout, fmt.Sprintf("could not list models: %v", err))
			models = nil
		}
		if draft.AI.PrimaryModel, err = p.askModelChoice("Primary model", models, draft.AI.PrimaryModel, false); err != nil {
			return err
		}
		if draft.AI.FallbackModel, err = p.askModelChoice("Fallback model", models, draft.AI.FallbackModel, true); err != nil {
			return err
		}
	}

	clitheme.Section(stdout, "Runtime behavior")
	clitheme.Muted(stdout, "Watch folders are monitored live. Scan frequency is a periodic catch-up for")
	clitheme.Muted(stdout, "anything the watcher missed (default 5m is fine for most libraries).")
	clitheme.Muted(stdout, "Move deletes the download after a successful import; copy keeps the source.")
	clitheme.Muted(stdout, "Checksums add an integrity check after each transfer (slower; off by default).")
	if draft.Runtime.ScanFrequency, err = p.askScanFrequency(firstNonEmpty(draft.Runtime.ScanFrequency, "5m")); err != nil {
		return err
	}
	if draft.Runtime.DeleteSource, err = p.askBool("Move files (delete the source after import)?", draft.Runtime.DeleteSource); err != nil {
		return err
	}
	if draft.Runtime.VerifyChecksums, err = p.askBool("Verify checksums after transfers?", draft.Runtime.VerifyChecksums); err != nil {
		return err
	}
	if deps.runtime.Kind == setupdomain.RuntimeContainer {
		clitheme.Muted(stdout, fmt.Sprintf("Container runtime detected (uid %d, gid %d): ownership is controlled by PUID/PGID.", deps.runtime.UID, deps.runtime.GID))
		draft.Runtime.Permissions = config.PermissionsConfig{}
	} else {
		// Same model as the TUI installer: group is what Sonarr/Radarr/Jellyfin
		// need for upgrades/deletes; user may stay empty (daemon/root owns).
		defs := deps.permDefaults()
		if ms := setupdomain.DetectedMediaServerName(); ms != "" {
			clitheme.Muted(stdout, fmt.Sprintf("Detected media server user/group defaults from %s.", ms))
		}
		clitheme.Muted(stdout, "Group is the critical setting: Sonarr, Radarr, and Jellyfin need a shared")
		clitheme.Muted(stdout, "group (and group-writable modes) to rename/upgrade/delete after import.")
		clitheme.Muted(stdout, "User may be blank (daemon/root owns) or your media server user.")
		clitheme.Muted(stdout, "Recommended: group=media (or jellyfin), file_mode=0664, dir_mode=0775.")
		clitheme.Muted(stdout, "Full guide: https://github.com/Nomadcxx/plex2jellyfin/blob/main/docs/permissions.md")
		for {
			draft.Runtime.Permissions.User, err = p.ask("Owner username (blank = leave owner as-is)", firstNonEmpty(draft.Runtime.Permissions.User, defs.User))
			if err != nil {
				return err
			}
			u := strings.ToLower(strings.TrimSpace(draft.Runtime.Permissions.User))
			if u == "y" || u == "n" || u == "yes" || u == "no" {
				fmt.Fprintln(stdout, "  that looks like a yes/no answer — enter a username (e.g. jellyfin), or leave blank")
				draft.Runtime.Permissions.User = ""
				continue
			}
			break
		}
		if draft.Runtime.Permissions.Group, err = p.ask("Owner group (shared group for multi-app access)", firstNonEmpty(draft.Runtime.Permissions.Group, defs.Group)); err != nil {
			return err
		}
		if g := draft.Runtime.Permissions.Group; g != "" {
			if who := setupdomain.ActualUsername(); who != "" && !setupdomain.UserInGroup(who, g) {
				clitheme.Warn(stdout, fmt.Sprintf("%s is not in group %q — CLI deletes/renames may fail", who, g))
				clitheme.Muted(stdout, fmt.Sprintf("Fix: sudo usermod -aG %s %s  (then re-login)", g, who))
			}
		}
		if draft.Runtime.Permissions.FileMode, err = p.askOctalMode("File mode", firstNonEmpty(draft.Runtime.Permissions.FileMode, defs.FileMode)); err != nil {
			return err
		}
		if draft.Runtime.Permissions.DirMode, err = p.askOctalMode("Directory mode", firstNonEmpty(draft.Runtime.Permissions.DirMode, defs.DirMode)); err != nil {
			return err
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

	clitheme.Section(stdout, "Review")
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
		clitheme.Err(stdout, fmt.Sprintf("Daemon activation failed: %v", err))
		clitheme.Muted(stdout, "The configuration is saved; re-run 'plex2jellyfin setup' or start the daemon service and it will pick it up.")
		return errors.New("daemon activation failed")
	}

	candidate.Setup.Completed = true
	if err := candidate.Save(); err != nil {
		return fmt.Errorf("mark setup complete: %w", err)
	}

	// Start Web UI before the Jellyfin feedback-loop verify (webhook hits :5522).
	webStarted := false
	if deps.runtime.Kind == setupdomain.RuntimeContainer {
		clitheme.Muted(stdout, "Container runtime: start the web UI container/service yourself if you need :5522.")
	} else if deps.webUnitOK != nil && !deps.webUnitOK() {
		clitheme.Warn(stdout, "plex2jellyfin-web systemd unit not installed; skipping Web UI start")
		clitheme.Muted(stdout, "Install via the TUI installer, then: sudo systemctl enable --now plex2jellyfin-web")
	} else {
		clitheme.Muted(stdout, "The Web UI serves the dashboard and Jellyfin webhook receiver on :5522.")
		startWeb, err := p.askBool("Enable and start the Web UI now?", true)
		if err != nil {
			return err
		}
		if startWeb {
			fmt.Fprintln(stdout, "Starting Web UI…")
			if err := deps.activateWeb(); err != nil {
				clitheme.Warn(stdout, fmt.Sprintf("Web UI start failed: %v", err))
				printWebServiceSkipHints(stdout)
			} else {
				webStarted = true
				clitheme.OK(stdout, "Web UI enabled and started")
			}
		} else {
			printWebServiceSkipHints(stdout)
		}
	}

	if pluginState.loaded {
		clitheme.Section(stdout, "Feedback loop")
		if deps.runtime.Kind == setupdomain.RuntimeContainer {
			clitheme.Muted(stdout, "plex2jellyfin runs in a container: Jellyfin most likely reaches it via")
			clitheme.Muted(stdout, "the Docker gateway IP (often 172.17.0.1) or a compose service name,")
			clitheme.Muted(stdout, "not the LAN IP suggested below.")
		}
		engine := deps.pluginEngine(candidate.Jellyfin.URL, candidate.Jellyfin.APIKey)
		pd := pluginDeps{
			loadConfig:  deps.loadConfig,
			saveConfig:  deps.saveConfig,
			newEngine:   func(string, string) pluginEngine { return engine },
			advertiseIP: deps.advertiseIP,
		}
		if err := configureAndVerify(ctx, pd, candidate, engine, p, stdout); err != nil {
			clitheme.Warn(stdout, fmt.Sprintf("feedback-loop setup incomplete: %v", err))
			clitheme.Muted(stdout, "finish it later with: plex2jellyfin plugin verify")
		}
	} else if pluginState.attempted {
		clitheme.Muted(stdout, "Companion plugin: restart Jellyfin, then run: plex2jellyfin plugin verify")
	} else if candidate.Jellyfin.Enabled && pluginState.skipped != "" && pluginState.skipped != "jellyfin disabled" {
		clitheme.Muted(stdout, "Companion plugin not installed - run: plex2jellyfin plugin install")
	}

	// Always index libraries at finish (TUI installer parity), then chown.
	if deps.initialScan != nil {
		if err := deps.initialScan(ctx, stdout, draft); err != nil {
			clitheme.Warn(stdout, fmt.Sprintf("Initial scan failed: %v", err))
			clitheme.Muted(stdout, "Re-run later with: plex2jellyfin scan")
		}
	}

	if webStarted {
		clitheme.OK(stdout, "Setup complete. Daemon and Web UI are running.")
		clitheme.Muted(stdout, "Open the web UI on :5522 when you are ready.")
	} else {
		clitheme.OK(stdout, "Setup complete. The daemon is running.")
	}
	return nil
}

// wizardPluginState carries what the Jellyfin-step plugin actions
// accomplished into the post-activation feedback-loop step.
type wizardPluginState struct {
	attempted bool   // user said yes to installing
	loaded    bool   // plugin responded after install(+restart)
	skipped   string // non-empty: reason the step was skipped, for the summary
}

// wizardPluginStep runs install/restart (engine stages 1-4) inside the
// Jellyfin step. Every failure degrades to a printed recovery command;
// nothing here can abort the wizard.
func wizardPluginStep(ctx context.Context, p *prompter, out io.Writer, deps setupDeps, jf setupdomain.ServiceDraft) wizardPluginState {
	if !jf.Enabled {
		return wizardPluginState{skipped: "jellyfin disabled"}
	}
	engine := deps.pluginEngine(jf.URL, jf.APIKey)
	insp, err := engine.Inspect(ctx)
	if err != nil {
		fmt.Fprintf(out, "  skipping the companion plugin step (%v)\n", err)
		fmt.Fprintln(out, "  install it later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "jellyfin unreachable"}
	}
	if !insp.ABISupported {
		fmt.Fprintf(out, "  Jellyfin %s cannot load the companion plugin (needs 10.11.x); see the docs for manual options\n", insp.ServerVersion)
		return wizardPluginState{skipped: "unsupported server"}
	}
	if insp.InstalledVersion != "" && insp.PluginResponding {
		fmt.Fprintf(out, "  companion plugin %s already installed\n", insp.InstalledVersion)
		return wizardPluginState{attempted: true, loaded: true}
	}

	fmt.Fprintln(out, "\nThe companion plugin is required for the feedback loop: it confirms")
	fmt.Fprintln(out, "organized files against real Jellyfin items and powers orphan detection.")
	install, err := p.askBool("Install it through Jellyfin's plugin system now?", true)
	if err != nil || !install {
		fmt.Fprintln(out, "  install it later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "declined"}
	}

	if added, err := engine.RegisterRepo(ctx); err != nil {
		fmt.Fprintf(out, "  registering the plugin repository failed: %v\n", err)
		fmt.Fprintln(out, "  retry later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "repository registration failed"}
	} else if added {
		fmt.Fprintln(out, "  plugin repository registered (existing repositories kept)")
	}
	if err := engine.Install(ctx); err != nil {
		fmt.Fprintf(out, "  plugin install failed: %v\n", err)
		fmt.Fprintln(out, "  retry later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "install failed"}
	}
	fmt.Fprintln(out, "  Jellyfin downloaded the plugin; a restart is needed before it loads")

	restart, err := p.askBool("Restart Jellyfin now?", false)
	if err != nil || !restart {
		fmt.Fprintln(out, "  after restarting Jellyfin, run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	if err := engine.Restart(ctx); err != nil {
		fmt.Fprintf(out, "  restart request failed: %v\n", err)
		fmt.Fprintln(out, "  restart Jellyfin manually, then run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	fmt.Fprintln(out, "  restart requested, waiting for the plugin to come back…")
	if err := engine.WaitReady(ctx, restartWaitTimeout); err != nil {
		fmt.Fprintf(out, "  Jellyfin did not come back in time (%v)\n", err)
		fmt.Fprintln(out, "  once it is up, run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	fmt.Fprintln(out, "  plugin loaded")
	return wizardPluginState{attempted: true, loaded: true}
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
		clitheme.Err(p.out, fmt.Sprintf("connection failed: %v", err))
		retry, rerr := p.askBool("Edit "+name+" settings and retry?", true)
		if rerr != nil {
			return rerr
		}
		if retry {
			return promptService(p, name, defaultURL, svc, test)
		}
		clitheme.Warn(p.out, fmt.Sprintf("keeping %s settings without a passing test", name))
		return nil
	}
	clitheme.OK(p.out, name+" connection OK")
	return nil
}

// promptArrCompatibility reports incompatible arr settings and only changes
// them after an explicit confirmation.
func promptArrCompatibility(p *prompter, out io.Writer, name string, svc setupdomain.ServiceDraft,
	check func(url, apiKey string) ([]service.HealthIssue, error),
	fix func(url, apiKey string, issues []service.HealthIssue) error) error {
	issues, err := check(svc.URL, svc.APIKey)
	if err != nil {
		clitheme.Warn(out, fmt.Sprintf("%s compatibility check failed: %v", name, err))
		return nil
	}
	if len(issues) == 0 {
		clitheme.OK(out, name+" settings are compatible")
		return nil
	}
	clitheme.Warn(out, fmt.Sprintf("%s has %d incompatible setting(s):", name, len(issues)))
	for _, issue := range issues {
		clitheme.Muted(out, fmt.Sprintf("%s: %s (currently %s, needs %s)", issue.Severity, issue.Setting, issue.Current, issue.Expected))
	}
	apply, err := p.askBool("Apply these "+name+" fixes now?", false)
	if err != nil {
		return err
	}
	if !apply {
		clitheme.Muted(out, fmt.Sprintf("leaving %s unchanged; run 'plex2jellyfin health --fix' later", name))
		return nil
	}
	if err := fix(svc.URL, svc.APIKey, issues); err != nil {
		clitheme.Err(out, fmt.Sprintf("fixing %s failed: %v", name, err))
		return nil
	}
	clitheme.OK(out, name+" updated")
	return nil
}

func printFieldErrors(out io.Writer, errs []setupdomain.FieldError) {
	clitheme.Err(out, "Setup cannot continue:")
	for _, e := range errs {
		clitheme.Muted(out, fmt.Sprintf("%s: %s", e.Field, e.Message))
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
	if d.Runtime.Permissions.WantsOwnership() || d.Runtime.Permissions.WantsMode() {
		owner := d.Runtime.Permissions.User
		if owner == "" {
			owner = "(unchanged)"
		}
		group := d.Runtime.Permissions.Group
		if group == "" {
			group = "(unchanged)"
		}
		line("Ownership", owner+":"+group)
		if d.Runtime.Permissions.FileMode != "" || d.Runtime.Permissions.DirMode != "" {
			line("Modes", firstNonEmpty(d.Runtime.Permissions.FileMode, "-")+" files / "+firstNonEmpty(d.Runtime.Permissions.DirMode, "-")+" dirs")
		}
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
