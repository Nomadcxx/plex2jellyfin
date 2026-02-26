package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/notify"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	backendName string
	healthAddr  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "jellywatchd",
		Short: "JellyWatch daemon service",
		Long: `JellyWatchd runs in the background monitoring directories for new media files.
It automatically organizes them according to Jellyfin naming conventions.`,
		RunE: runDaemon,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&backendName, "backend", "auto", "transfer backend: auto, pv, rsync, native")
	rootCmd.PersistentFlags().StringVar(&healthAddr, "health-addr", ":8686", "health check server address")

	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newUninstallCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDaemon(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("unable to load config: %w", err)
	}

	logCfg := logging.Config{
		Level:      cfg.Logging.Level,
		File:       cfg.Logging.File,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
	}
	logger, err := logging.New(logCfg)
	if err != nil {
		return fmt.Errorf("unable to create logger: %w", err)
	}
	defer logger.Close()

	var watchPaths []string
	watchPaths = append(watchPaths, cfg.Watch.Movies...)
	watchPaths = append(watchPaths, cfg.Watch.TV...)

	if len(watchPaths) == 0 {
		return fmt.Errorf("no watch directories configured")
	}

	notifyMgr := notify.NewManager(true)

	var targetUID, targetGID int = -1, -1
	var fileMode, dirMode os.FileMode
	if cfg.Permissions.WantsOwnership() {
		var err error
		targetUID, err = cfg.Permissions.ResolveUID()
		if err != nil {
			return fmt.Errorf("invalid permissions user %q: %w", cfg.Permissions.User, err)
		}
		targetGID, err = cfg.Permissions.ResolveGID()
		if err != nil {
			return fmt.Errorf("invalid permissions group %q: %w", cfg.Permissions.Group, err)
		}
		logger.Info("daemon", "File ownership configured",
			logging.F("uid", targetUID),
			logging.F("gid", targetGID))

		if os.Geteuid() != 0 {
			logger.Error("daemon", "Permission ownership configured but daemon not running as root",
				fmt.Errorf("chown will fail"),
				logging.F("current_euid", os.Geteuid()),
				logging.F("target_uid", targetUID),
				logging.F("target_gid", targetGID),
				logging.F("fix", "Update systemd service to run as root: User=root"))
			return fmt.Errorf("daemon must run as root to set file ownership (current euid=%d)", os.Geteuid())
		}
	}
	if cfg.Permissions.WantsMode() {
		var err error
		fileMode, err = cfg.Permissions.ParseFileMode()
		if err != nil {
			return fmt.Errorf("invalid permissions file_mode %q: %w", cfg.Permissions.FileMode, err)
		}
		dirMode, err = cfg.Permissions.ParseDirMode()
		if err != nil {
			return fmt.Errorf("invalid permissions dir_mode %q: %w", cfg.Permissions.DirMode, err)
		}
		logger.Info("daemon", "File permissions configured",
			logging.F("file_mode", fmt.Sprintf("%04o", fileMode)),
			logging.F("dir_mode", fmt.Sprintf("%04o", dirMode)))
	}

	// Create Sonarr client (used for both notifications AND library selection)
	var sonarrClient *sonarr.Client
	if cfg.Sonarr.Enabled && cfg.Sonarr.APIKey != "" && cfg.Sonarr.URL != "" {
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 30 * time.Second,
		})

		// Test connection
		if err := sonarrClient.Ping(); err != nil {
			logger.Warn("daemon", "Sonarr connection failed, will continue without intelligent library selection",
				logging.F("error", err.Error()))
			sonarrClient = nil // Don't use if connection fails
		} else {
			notifyMgr.Register(notify.NewSonarrNotifier(sonarrClient, cfg.Sonarr.NotifyOnImport))
			logger.Info("daemon", "Sonarr integration enabled", logging.F("url", cfg.Sonarr.URL))
		}
	}

	if cfg.Radarr.Enabled && cfg.Radarr.APIKey != "" && cfg.Radarr.URL != "" {
		radarrClient := radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 30 * time.Second,
		})
		notifyMgr.Register(notify.NewRadarrNotifier(radarrClient, cfg.Radarr.NotifyOnImport))
		logger.Info("daemon", "Radarr integration enabled", logging.F("url", cfg.Radarr.URL))
	}

	var jellyfinClient *jellyfin.Client
	if cfg.Jellyfin.Enabled && cfg.Jellyfin.APIKey != "" && cfg.Jellyfin.URL != "" {
		jellyfinClient = jellyfin.NewClient(jellyfin.Config{
			URL:     cfg.Jellyfin.URL,
			APIKey:  cfg.Jellyfin.APIKey,
			Timeout: 30 * time.Second,
		})
		if err := jellyfinClient.Ping(); err != nil {
			logger.Warn("daemon", "Jellyfin connection failed, disabling integration", logging.F("error", err.Error()))
			jellyfinClient = nil
		} else {
			info, err := jellyfinClient.GetSystemInfo()
			if err == nil && info != nil {
				logger.Info("daemon", "Jellyfin integration enabled",
					logging.F("server", info.ServerName),
					logging.F("version", info.Version))
			} else {
				logger.Info("daemon", "Jellyfin integration enabled", logging.F("url", cfg.Jellyfin.URL))
			}
			notifyMgr.Register(notify.NewJellyfinNotifier(cfg.Jellyfin.URL, cfg.Jellyfin.APIKey, cfg.Jellyfin.NotifyOnImport))
		}
	}

	var playbackLocks *jellyfin.PlaybackLockManager
	var deferredQueue *jellyfin.DeferredQueue
	if cfg.Jellyfin.PlaybackSafety {
		playbackLocks = jellyfin.NewPlaybackLockManager()
		deferredQueue = jellyfin.NewDeferredQueue()
		logger.Info("daemon", "Jellyfin playback safety enabled")
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	// Get config directory for activity logging
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
	if cfgFile != "" {
		configDir = filepath.Dir(cfgFile)
	}

	handler, err := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
		TVLibraries:     cfg.Libraries.TV,
		MovieLibs:       cfg.Libraries.Movies,
		TVWatchPaths:    cfg.Watch.TV,
		MovieWatchPaths: cfg.Watch.Movies,
		DebounceTime:    10 * time.Second,
		DryRun:          false, // Daemon always processes files automatically
		Timeout:         5 * time.Minute,
		Backend:         transfer.ParseBackend(backendName),
		NotifyManager:   notifyMgr,
		Logger:          logger,
		TargetUID:       targetUID,
		TargetGID:       targetGID,
		FileMode:        fileMode,
		DirMode:         dirMode,
		SonarrClient:    sonarrClient,
		JellyfinClient:  jellyfinClient,
		PlaybackSafety:  cfg.Jellyfin.PlaybackSafety,
		Database:        db,
		ConfigDir:       configDir,
		PlaybackLocks:   playbackLocks,
		DeferredQueue:   deferredQueue,
	})
	if err != nil {
		return fmt.Errorf("failed to create media handler: %w", err)
	}

	// Prune old activity logs (keep 7 days)
	if err := handler.PruneActivityLogs(7); err != nil {
		logger.Warn("daemon", "Failed to prune old activity logs", logging.F("error", err.Error()))
	}

	// Parse scan frequency
	scanInterval, err := time.ParseDuration(cfg.Daemon.ScanFrequency)
	if err != nil {
		logger.Warn("daemon", "Invalid scan_frequency, using default",
			logging.F("configured", cfg.Daemon.ScanFrequency),
			logging.F("default", "5m"))
		scanInterval = 5 * time.Minute
	}

	// Create periodic scanner
	periodicScanner := scanner.NewPeriodicScanner(scanner.ScannerConfig{
		Interval:    scanInterval,
		WatchPaths:  watchPaths,
		Handler:     handler,
		Logger:      logger,
		ActivityDir: filepath.Join(configDir, "activity"),
	})

	healthServer := daemon.NewServer(handler, periodicScanner, healthAddr, logger, cfg.Jellyfin.WebhookSecret)

	w, err := watcher.NewWatcher(handler, false) // Daemon always processes files automatically
	if err != nil {
		return fmt.Errorf("unable to create watcher: %w", err)
	}
	defer w.Close()

	if err := w.Watch(watchPaths); err != nil {
		return fmt.Errorf("unable to watch directories: %w", err)
	}

	// Perform initial scan of existing files
	logger.Info("daemon", "Performing initial scan of existing files")
	if err := performInitialScan(handler, watchPaths, logger); err != nil {
		logger.Warn("daemon", "Initial scan completed with errors", logging.F("error", err.Error()))
	} else {
		logger.Info("daemon", "Initial scan completed successfully")
	}

	logger.Info("daemon", "JellyWatchd started",
		logging.F("watch_dirs", len(watchPaths)),
		logging.F("tv_libs", cfg.Libraries.TV),
		logging.F("movie_libs", cfg.Libraries.Movies),
		logging.F("health_addr", healthAddr),
		logging.F("log_file", logger.FilePath()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	errChan := make(chan error, 3)

	go func() {
		errChan <- w.Start()
	}()

	go func() {
		errChan <- healthServer.Start()
	}()

	go func() {
		errChan <- periodicScanner.Start(ctx)
	}()

	select {
	case sig := <-sigChan:
		logger.Info("daemon", "Received shutdown signal", logging.F("signal", sig.String()))
		healthServer.SetHealthy(false)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		healthServer.Shutdown(shutdownCtx)
		handler.Shutdown()
		cancel()
		return nil

	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("service error: %w", err)
		}
		return nil

	case <-ctx.Done():
		return nil
	}
}

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install jellywatchd as a systemd service",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("To install jellywatchd as a systemd service:")
			fmt.Println()
			fmt.Println("1. Copy the binary:")
			fmt.Println("   sudo cp jellywatchd /usr/local/bin/")
			fmt.Println()
			fmt.Println("2. Copy the service file:")
			fmt.Println("   sudo cp jellywatchd.service /etc/systemd/system/")
			fmt.Println()
			fmt.Println("3. Reload systemd:")
			fmt.Println("   sudo systemctl daemon-reload")
			fmt.Println()
			fmt.Println("4. Enable and start:")
			fmt.Println("   sudo systemctl enable jellywatchd")
			fmt.Println("   sudo systemctl start jellywatchd")
			fmt.Println()
			fmt.Println("5. Check status:")
			fmt.Println("   sudo systemctl status jellywatchd")
			fmt.Println("   journalctl -u jellywatchd -f")
		},
	}
}

// isMediaFile checks if a path represents a media file that should be processed
func isMediaFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	mediaExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	}
	return mediaExts[ext]
}

// performInitialScan walks through all watch directories and processes any existing media files
func performInitialScan(handler *daemon.MediaHandler, watchPaths []string, logger *logging.Logger) error {
	totalProcessed := 0
	totalErrors := 0

	for _, watchPath := range watchPaths {
		logger.Info("daemon", "Scanning directory for existing files", logging.F("path", watchPath))

		err := filepath.Walk(watchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				logger.Warn("daemon", "Directory inaccessible during scan",
					logging.F("path", path),
					logging.F("error", err.Error()),
					logging.F("suggestion", "Check permissions: chown $USER:media "+path))
				return nil // Continue scanning other directories
			}

			if !info.IsDir() && handler.IsMediaFile(path) {
				logger.Info("daemon", "Processing existing file", logging.F("file", filepath.Base(path)))

				// Create a file event for the existing file
				event := watcher.FileEvent{
					Type: watcher.EventCreate, // Treat as new file
					Path: path,
				}

				if err := handler.HandleFileEvent(event); err != nil {
					logger.Error("daemon", "Failed to process existing file", err, logging.F("file", path))
					totalErrors++
				} else {
					totalProcessed++
				}
			}

			return nil
		})

		if err != nil {
			logger.Warn("daemon", "Error scanning directory", logging.F("path", watchPath), logging.F("error", err.Error()))
		}
	}

	logger.Info("daemon", "Initial scan summary",
		logging.F("processed", totalProcessed),
		logging.F("errors", totalErrors))

	return nil
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall jellywatchd systemd service",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("To uninstall jellywatchd:")
			fmt.Println()
			fmt.Println("1. Stop and disable:")
			fmt.Println("   sudo systemctl stop jellywatchd")
			fmt.Println("   sudo systemctl disable jellywatchd")
			fmt.Println()
			fmt.Println("2. Remove files:")
			fmt.Println("   sudo rm /etc/systemd/system/jellywatchd.service")
			fmt.Println("   sudo rm /usr/local/bin/jellywatchd")
			fmt.Println()
			fmt.Println("3. Reload systemd:")
			fmt.Println("   sudo systemctl daemon-reload")
		},
	}
}
