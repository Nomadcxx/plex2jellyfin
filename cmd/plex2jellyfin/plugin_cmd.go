package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/spf13/cobra"
)

// pluginEngine is the slice of plugininstall.Engine the commands use,
// injectable for tests.
type pluginEngine interface {
	Inspect(ctx context.Context) (*plugininstall.Inspection, error)
	RegisterRepo(ctx context.Context) (bool, error)
	Install(ctx context.Context) error
	Restart(ctx context.Context) error
	WaitReady(ctx context.Context, timeout time.Duration) error
	Configure(ctx context.Context, daemonBaseURL, sharedSecret string) error
	Verify(ctx context.Context) (*plugininstall.VerifyResult, error)
}

type pluginDeps struct {
	loadConfig  func() (*config.Config, error)
	saveConfig  func(*config.Config) error
	newEngine   func(baseURL, apiKey string) pluginEngine
	advertiseIP func() string
}

func defaultPluginDeps() pluginDeps {
	return pluginDeps{
		loadConfig: config.Load,
		saveConfig: func(c *config.Config) error { return c.Save() },
		newEngine: func(baseURL, apiKey string) pluginEngine {
			return plugininstall.New(baseURL, apiKey, &http.Client{Timeout: 15 * time.Second})
		},
		advertiseIP: setupdomain.DetectAdvertiseIP,
	}
}

const restartWaitTimeout = 60 * time.Second

func newPluginCmd() *cobra.Command {
	deps := defaultPluginDeps()
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Install and verify the companion Jellyfin plugin",
		Long: `Manages the companion Jellyfin plugin through Jellyfin's own plugin
system: registers the plugin repository, installs the package, restarts
Jellyfin (with consent), pushes the webhook configuration, and verifies
the feedback loop with a signed test event.`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install, configure, and verify the plugin",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd.Context(), deps, os.Stdin, os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Send a signed test event through Jellyfin and confirm receipt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginVerify(cmd.Context(), deps, os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show plugin installation state on the configured Jellyfin",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginStatus(cmd.Context(), deps, os.Stdout)
		},
	})
	return cmd
}

func pluginEngineFromConfig(deps pluginDeps) (*config.Config, pluginEngine, error) {
	cfg, err := deps.loadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	if !cfg.Jellyfin.Enabled || cfg.Jellyfin.URL == "" || cfg.Jellyfin.APIKey == "" {
		return nil, nil, errors.New("Jellyfin is not configured - run 'plex2jellyfin setup' first")
	}
	return cfg, deps.newEngine(cfg.Jellyfin.URL, cfg.Jellyfin.APIKey), nil
}

func runPluginInstall(ctx context.Context, deps pluginDeps, stdin io.Reader, out io.Writer) error {
	cfg, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	p := &prompter{in: bufio.NewScanner(stdin), out: out}

	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	if !insp.ABISupported {
		return fmt.Errorf("Jellyfin %s is not supported: the plugin needs 10.11.x", insp.ServerVersion)
	}

	if insp.InstalledVersion != "" && insp.PluginResponding {
		fmt.Fprintf(out, "Plugin %s is already installed and responding.\n", insp.InstalledVersion)
	} else {
		if added, err := engine.RegisterRepo(ctx); err != nil {
			return fmt.Errorf("register plugin repository: %w", err)
		} else if added {
			fmt.Fprintln(out, "  plugin repository registered (existing repositories kept)")
		} else {
			fmt.Fprintln(out, "  plugin repository already registered")
		}
		if err := engine.Install(ctx); err != nil {
			return fmt.Errorf("install plugin package: %w", err)
		}
		fmt.Fprintln(out, "  Jellyfin downloaded the plugin package")

		restart, err := p.askBool("Restart Jellyfin now to load it?", false)
		if err != nil {
			return err
		}
		if !restart {
			fmt.Fprintln(out, "Restart Jellyfin when convenient, then run: plex2jellyfin plugin verify")
			return nil
		}
		if err := engine.Restart(ctx); err != nil {
			return fmt.Errorf("restart Jellyfin: %w", err)
		}
		fmt.Fprintln(out, "  restart requested, waiting for the plugin to come back…")
		if err := engine.WaitReady(ctx, restartWaitTimeout); err != nil {
			fmt.Fprintf(out, "Jellyfin did not come back in time (%v).\n", err)
			fmt.Fprintln(out, "Once it is up again, run: plex2jellyfin plugin verify")
			return nil
		}
		fmt.Fprintln(out, "  plugin loaded")
	}

	return configureAndVerify(ctx, deps, cfg, engine, p, out)
}

// configureAndVerify pushes the webhook settings into the plugin and
// proves the reverse path with a signed test event. Shared by the
// standalone command and the setup wizard's feedback-loop step.
func configureAndVerify(ctx context.Context, deps pluginDeps, cfg *config.Config, engine pluginEngine, p *prompter, out io.Writer) error {
	secret := firstNonEmpty(cfg.Jellyfin.PluginSharedSecret, cfg.Jellyfin.WebhookSecret)
	if secret == "" {
		fmt.Fprintln(out, "No webhook secret in the config - run 'plex2jellyfin setup' to generate one.")
		return nil
	}

	daemonURL := cfg.Jellyfin.PluginDaemonURL
	if daemonURL == "" {
		def := "http://localhost:5522"
		if ip := deps.advertiseIP(); ip != "" {
			def = "http://" + ip + ":5522"
		}
		var err error
		if daemonURL, err = p.ask("Web UI URL reachable from Jellyfin", def); err != nil {
			return err
		}
		cfg.Jellyfin.PluginDaemonURL = strings.TrimRight(daemonURL, "/")
		cfg.Jellyfin.PluginEnabled = true
		if err := deps.saveConfig(cfg); err != nil {
			return fmt.Errorf("save plugin daemon URL: %w", err)
		}
	}

	if err := engine.Configure(ctx, daemonURL, secret); err != nil {
		fmt.Fprintf(out, "Pushing the plugin configuration failed: %v\n", err)
		fmt.Fprintln(out, "Retry later with: plex2jellyfin plugin verify")
		return nil
	}
	fmt.Fprintln(out, "  plugin configured (webhook URL + secret pushed)")

	res, err := engine.Verify(ctx)
	if err != nil {
		fmt.Fprintf(out, "Verification call failed: %v\n", err)
		fmt.Fprintln(out, "Retry later with: plex2jellyfin plugin verify")
		return nil
	}
	printVerifyResult(out, res)
	return nil
}

func printVerifyResult(out io.Writer, res *plugininstall.VerifyResult) {
	switch {
	case res.Sent && res.Authenticated:
		fmt.Fprintln(out, "  verified - a signed test event round-tripped through Jellyfin")
	case res.Sent:
		fmt.Fprintf(out, "  test event reached %s but got HTTP %d - check that the webhook secret matches\n",
			res.DaemonURL, res.DaemonStatusCode)
	default:
		fmt.Fprintf(out, "  Jellyfin could not reach %s: %s\n", res.DaemonURL, res.Error)
		fmt.Fprintln(out, "  (wrong URL for Jellyfin's network? containers cannot reach the host's localhost)")
	}
}

func runPluginVerify(ctx context.Context, deps pluginDeps, out io.Writer) error {
	_, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	if !insp.PluginResponding {
		return errors.New("the plugin is not responding - is it installed and Jellyfin restarted? Run: plex2jellyfin plugin install")
	}
	res, err := engine.Verify(ctx)
	if err != nil {
		return fmt.Errorf("trigger test event: %w", err)
	}
	printVerifyResult(out, res)
	if !res.Sent || !res.Authenticated {
		return errors.New("feedback loop verification failed")
	}
	return nil
}

func runPluginStatus(ctx context.Context, deps pluginDeps, out io.Writer) error {
	_, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	fmt.Fprintf(out, "Jellyfin server   %s (supported: %v)\n", insp.ServerVersion, insp.ABISupported)
	fmt.Fprintf(out, "Plugin repository %v\n", insp.RepoRegistered)
	if insp.InstalledVersion == "" {
		fmt.Fprintln(out, "Plugin            not installed - run: plex2jellyfin plugin install")
	} else {
		fmt.Fprintf(out, "Plugin            %s (responding: %v)\n", insp.InstalledVersion, insp.PluginResponding)
	}
	return nil
}
