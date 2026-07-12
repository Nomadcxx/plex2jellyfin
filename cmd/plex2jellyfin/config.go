package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Plex2Jellyfin configuration",
		Long: `Commands for managing Plex2Jellyfin configuration.

The config file is stored at: ~/.config/plex2jellyfin/config.toml

Examples:
  plex2jellyfin config init              # Create default config file
  plex2jellyfin config show              # Display current configuration
  plex2jellyfin config test              # Test all connections
  plex2jellyfin config path              # Show config file path`,
	}

	cmd.AddCommand(newConfigInitCmd())
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigTestCmd())
	cmd.AddCommand(newConfigPathCmd())

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		Long: `Create a new configuration file with default values.

The config file will be created at ~/.config/plex2jellyfin/config.toml
Edit this file to set your watch directories, library paths, and API keys.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if config.ConfigExists() && !force {
				path, _ := config.ConfigPath()
				return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
			}

			cfg := config.DefaultConfig()

			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			path, _ := config.ConfigPath()
			fmt.Printf("[ok] Created config file: %s\n", path)
			fmt.Println("\nNext steps:")
			fmt.Println("  1. Edit the config file to set your paths and API keys")
			fmt.Println("  2. Run 'plex2jellyfin config test' to verify connections")
			fmt.Println("  3. Run 'plex2jellyfin config show' to review settings")

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config file")

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !config.ConfigExists() {
				return fmt.Errorf("no config file found (run 'plex2jellyfin config init' first)")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			path, _ := config.ConfigPath()
			fmt.Printf("Config file: %s\n\n", path)

			fmt.Println("=== Watch Directories ===")
			fmt.Printf("TV:     %v\n", cfg.Watch.TV)
			fmt.Printf("Movies: %v\n", cfg.Watch.Movies)

			fmt.Println("\n=== Library Directories ===")
			fmt.Printf("TV:     %v\n", cfg.Libraries.TV)
			fmt.Printf("Movies: %v\n", cfg.Libraries.Movies)

			fmt.Println("\n=== Sonarr ===")
			fmt.Printf("Enabled: %v\n", cfg.Sonarr.Enabled)
			if cfg.Sonarr.Enabled {
				fmt.Printf("URL:     %s\n", cfg.Sonarr.URL)
				fmt.Printf("API Key: %s\n", maskAPIKey(cfg.Sonarr.APIKey))
				fmt.Printf("Notify:  %v\n", cfg.Sonarr.NotifyOnImport)
			}

			fmt.Println("\n=== Radarr ===")
			fmt.Printf("Enabled: %v\n", cfg.Radarr.Enabled)
			if cfg.Radarr.Enabled {
				fmt.Printf("URL:     %s\n", cfg.Radarr.URL)
				fmt.Printf("API Key: %s\n", maskAPIKey(cfg.Radarr.APIKey))
				fmt.Printf("Notify:  %v\n", cfg.Radarr.NotifyOnImport)
			}

			fmt.Println("\n=== Options ===")
			fmt.Printf("Dry Run:         %v\n", cfg.Options.DryRun)
			fmt.Printf("Delete Source:   %v\n", cfg.Options.DeleteSource)
			fmt.Printf("Verify Checksum: %v\n", cfg.Options.VerifyChecksums)

			return nil
		},
	}
}

func newConfigTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test configuration and connections",
		Long: `Verify that all configured paths exist and API connections work.

Tests:
  - Watch directories are readable
  - Library directories are writable
  - Sonarr connection (if enabled)
  - Radarr connection (if enabled)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !config.ConfigExists() {
				return fmt.Errorf("no config file found (run 'plex2jellyfin config init' first)")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var errors []string
			var warnings []string

			fmt.Println("Testing configuration...")
			fmt.Println()

			fmt.Println("=== Watch Directories ===")
			if len(cfg.Watch.TV) == 0 && len(cfg.Watch.Movies) == 0 {
				warnings = append(warnings, "No watch directories configured")
				fmt.Println("[warn] No watch directories configured")
			}
			for _, dir := range cfg.Watch.TV {
				if err := testReadable(dir); err != nil {
					errors = append(errors, fmt.Sprintf("TV watch dir %s: %v", dir, err))
					fmt.Printf("[error] TV: %s (%v)\n", dir, err)
				} else {
					fmt.Printf("[ok] TV: %s\n", dir)
				}
			}
			for _, dir := range cfg.Watch.Movies {
				if err := testReadable(dir); err != nil {
					errors = append(errors, fmt.Sprintf("Movie watch dir %s: %v", dir, err))
					fmt.Printf("[error] Movies: %s (%v)\n", dir, err)
				} else {
					fmt.Printf("[ok] Movies: %s\n", dir)
				}
			}

			fmt.Println("\n=== Library Directories ===")
			if len(cfg.Libraries.TV) == 0 && len(cfg.Libraries.Movies) == 0 {
				warnings = append(warnings, "No library directories configured")
				fmt.Println("[warn] No library directories configured")
			}
			for _, dir := range cfg.Libraries.TV {
				if err := testWritable(dir); err != nil {
					errors = append(errors, fmt.Sprintf("TV library %s: %v", dir, err))
					fmt.Printf("[error] TV: %s (%v)\n", dir, err)
				} else {
					fmt.Printf("[ok] TV: %s\n", dir)
				}
			}
			for _, dir := range cfg.Libraries.Movies {
				if err := testWritable(dir); err != nil {
					errors = append(errors, fmt.Sprintf("Movie library %s: %v", dir, err))
					fmt.Printf("[error] Movies: %s (%v)\n", dir, err)
				} else {
					fmt.Printf("[ok] Movies: %s\n", dir)
				}
			}

			fmt.Println("\n=== Sonarr ===")
			if cfg.Sonarr.Enabled {
				if cfg.Sonarr.URL == "" {
					errors = append(errors, "Sonarr enabled but URL not set")
					fmt.Println("[error] URL not configured")
				} else if cfg.Sonarr.APIKey == "" {
					errors = append(errors, "Sonarr enabled but API key not set")
					fmt.Println("[error] API key not configured")
				} else {
					client := sonarr.NewClient(sonarr.Config{
						URL:     cfg.Sonarr.URL,
						APIKey:  cfg.Sonarr.APIKey,
						Timeout: 10 * time.Second,
					})
					status, err := client.GetSystemStatus()
					if err != nil {
						errors = append(errors, fmt.Sprintf("Sonarr connection failed: %v", err))
						fmt.Printf("[error] Connection failed: %v\n", err)
					} else {
						fmt.Printf("[ok] Connected to %s v%s\n", status.AppName, status.Version)
					}
				}
			} else {
				fmt.Println("[ ] Disabled")
			}

			fmt.Println("\n=== Radarr ===")
			if cfg.Radarr.Enabled {
				if cfg.Radarr.URL == "" {
					errors = append(errors, "Radarr enabled but URL not set")
					fmt.Println("[error] URL not configured")
				} else if cfg.Radarr.APIKey == "" {
					errors = append(errors, "Radarr enabled but API key not set")
					fmt.Println("[error] API key not configured")
				} else {
					client := radarr.NewClient(radarr.Config{
						URL:     cfg.Radarr.URL,
						APIKey:  cfg.Radarr.APIKey,
						Timeout: 10 * time.Second,
					})
					status, err := client.GetSystemStatus()
					if err != nil {
						errors = append(errors, fmt.Sprintf("Radarr connection failed: %v", err))
						fmt.Printf("[error] Connection failed: %v\n", err)
					} else {
						fmt.Printf("[ok] Connected to Radarr v%s\n", status.Version)
					}
				}
			} else {
				fmt.Println("[ ] Disabled")
			}

			fmt.Println("\n" + separator(40))
			if len(errors) > 0 {
				fmt.Printf("\n[error] %d error(s) found:\n", len(errors))
				for _, e := range errors {
					fmt.Printf("  - %s\n", e)
				}
				return fmt.Errorf("configuration has %d error(s)", len(errors))
			}

			if len(warnings) > 0 {
				fmt.Printf("\n[warn] %d warning(s):\n", len(warnings))
				for _, w := range warnings {
					fmt.Printf("  - %s\n", w)
				}
			}

			fmt.Println("\n[ok] Configuration is valid")
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			if !config.ConfigExists() {
				fmt.Println("(file does not exist)")
			}
			return nil
		},
	}
}

func testReadable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("does not exist")
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	_, err = os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("not readable")
	}
	return nil
}

func testWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("does not exist")
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	testFile := fmt.Sprintf("%s/.plex2jellyfin_write_test_%d", path, time.Now().UnixNano())
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("not writable")
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func separator(n int) string {
	return strings.Repeat("=", n)
}
