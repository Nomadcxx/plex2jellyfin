package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon"
	daemonipc "github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	daemonreload "github.com/Nomadcxx/jellywatch/internal/daemon/reload"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/housekeeping"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/labeling"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/notify"
	"github.com/Nomadcxx/jellywatch/internal/paths"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
	"github.com/Nomadcxx/jellywatch/internal/scheduler"
	"github.com/Nomadcxx/jellywatch/internal/tmdb"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	backendName string
	healthAddr  string
	version     = "dev" // Set by build flags: -ldflags="-X main.version=1.0.0"
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
	var currentConfigMu sync.RWMutex
	currentConfig := cfg
	getCurrentConfig := func() *config.Config {
		currentConfigMu.RLock()
		defer currentConfigMu.RUnlock()
		return currentConfig
	}
	setCurrentConfig := func(next *config.Config) {
		currentConfigMu.Lock()
		defer currentConfigMu.Unlock()
		currentConfig = next
	}

	logCfg := logging.Config{
		Level:      cfg.Logging.Level,
		File:       cfg.Logging.File,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAgeDays: cfg.Logging.MaxAgeDays,
		Compress:   cfg.Logging.Compress,
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

	// Warn if any watch path overlaps with a library path — this causes the
	// watcher to recursively monitor the entire library, generating massive
	// event volume and memory usage.
	allLibPaths := append(cfg.Libraries.Movies, cfg.Libraries.TV...)
	for _, wp := range watchPaths {
		wpClean := filepath.Clean(wp)
		for _, lp := range allLibPaths {
			lpClean := filepath.Clean(lp)
			if wpClean == lpClean || strings.HasPrefix(wpClean, lpClean+string(filepath.Separator)) || strings.HasPrefix(lpClean, wpClean+string(filepath.Separator)) {
				logger.Warn("daemon", "DANGEROUS: watch path overlaps with library path — this will cause excessive memory and CPU usage",
					logging.F("watch_path", wp),
					logging.F("library_path", lp))
			}
		}
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

	// Recover any sync_log entries stuck in "running" from previous crashes
	if recovered, err := db.RecoverStuckSyncLogs(1 * time.Hour); err != nil {
		logger.Warn("daemon", "Failed to recover stuck sync logs", logging.F("error", err.Error()))
	} else if recovered > 0 {
		logger.Info("daemon", "Recovered stuck sync log entries", logging.F("count", recovered))
	}

	// Get config directory for activity logging.  Use paths.JellyWatchDir
	// so we honour SUDO_USER and write to the same ~/.config/jellywatch
	// the web UI reads from (otherwise root-run daemon and SUDO_USER-aware
	// jellyweb diverge: daemon writes /root/.config/..., webui reads
	// /home/nomadx/.config/... and the activity feed is permanently empty).
	configDir, err := paths.JellyWatchDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
	}
	if cfgFile != "" {
		configDir = filepath.Dir(cfgFile)
	}

	// Create AI matcher for daemon enhancement if enabled
	var aiMatcher *ai.Matcher
	if cfg.AI.Enabled {
		var matcherErr error
		aiMatcher, matcherErr = ai.NewMatcher(cfg.AI)
		if matcherErr != nil {
			logger.Warn("daemon", "AI matcher initialization failed, daemon will use regex only",
				logging.F("error", matcherErr.Error()))
		} else {
			logger.Info("daemon", "AI enhancement enabled",
				logging.F("model", cfg.AI.Model),
				logging.F("hourly_limit", cfg.AI.HourlyLimit),
				logging.F("daily_limit", cfg.AI.DailyLimit))
		}
	}

	pathMappings := make([]jellyfin.PathMapping, 0, len(cfg.Jellyfin.PathMappings))
	for _, m := range cfg.Jellyfin.PathMappings {
		pathMappings = append(pathMappings, jellyfin.PathMapping{Jellyfin: m.Jellyfin, Daemon: m.Daemon})
	}
	pathTranslator := jellyfin.NewPathTranslator(pathMappings)
	if len(pathMappings) > 0 {
		logger.Info("daemon", "Jellyfin path mappings configured",
			logging.F("count", len(pathMappings)))
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
		PathTranslator:  pathTranslator,
		AIEnabled:       cfg.AI.Enabled && aiMatcher != nil,
		AIMatcher:       aiMatcher,
		AIConfig:        cfg.AI,
		TransferConcurrencyPerVolume: cfg.Options.TransferConcurrencyPerVolume,
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
		OrphanCheck: jellyfinClient,
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

	reloadSupervisor := daemonreload.NewSupervisor()
	reloadSupervisor.Register(daemonreload.NewLoggingReloadable(logger))
	reloadSupervisor.Register(daemonreload.NewScannerReloadable(w))
	if aiMatcher != nil {
		reloadSupervisor.Register(daemonreload.NewAIReloadable(aiMatcher))
	}

	controlServer := daemonipc.NewServer(filepath.Join(configDir, "control.sock"))

	// Op-registry + op-log are required by streaming commands. The server
	// already allocates an OpRegistry; we open the on-disk op log and read
	// any pending entries left over from a prior crash.
	opLogPath := filepath.Join(configDir, "op_log.jsonl")
	opLog, err := daemonipc.OpenOpLog(opLogPath)
	if err != nil {
		return fmt.Errorf("open op log: %w", err)
	}
	defer opLog.Close()

	pending, _ := opLog.Pending()
	var pendingMu sync.Mutex
	getPending := func() []daemonipc.OpLogEntry {
		pendingMu.Lock()
		defer pendingMu.Unlock()
		out := make([]daemonipc.OpLogEntry, len(pending))
		copy(out, pending)
		return out
	}
	clearPending := func() {
		pendingMu.Lock()
		pending = nil
		pendingMu.Unlock()
	}

	controlServer.Register(daemonipc.CmdStatus, statusHandler(time.Now(), getCurrentConfig, getPending, version))
	controlServer.Register(daemonipc.CmdReload, reloadHandler(getCurrentConfig, setCurrentConfig, config.Load, reloadSupervisor))
	controlServer.Register(daemonipc.CmdStop, stopHandler(func() { cancel() }))
	controlServer.Register(daemonipc.CmdRecover, recoverHandler(opLog, getPending, clearPending))
	controlServer.Register(daemonipc.CmdDeferred, deferredHandler(func() any {
		return handler.UnparseableCache().Snapshot()
	}))

	fileScanner := scanner.NewFileScanner(db)
	rescanDefaults := func() []string {
		paths := append([]string{}, cfg.Libraries.TV...)
		paths = append(paths, cfg.Libraries.Movies...)
		return paths
	}
	// Construct Jellyfin sweeper up front so the IPC streaming handler can
	// trigger it manually; the periodic ticker reuses the same instance.
	var jfSweeper *jellyfin.Sweeper
	if jellyfinClient != nil && db != nil {
		jfSweeper = jellyfin.NewSweeper(jellyfinClient, db)
		jfSweeper.SetPathTranslator(pathTranslator)
	}

	controlServer.RegisterStreaming(daemonipc.CmdRescan, guardMutator(getPending, rescanHandler(fileScanner, rescanDefaults, opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdResetDB, guardMutator(getPending, resetDBHandler(db.SQL(), opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdConsolidate, guardMutator(getPending, consolidateHandler(db, opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdDupScan, dupScanHandler(service.NewCleanupService(db), opLog))
	controlServer.RegisterStreaming(daemonipc.CmdAIBatch, guardMutator(getPending, aiBatchHandler(handler, aiMatcher, opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdMetadataRefresh, guardMutator(getPending, metadataRefreshHandler(jellyfinClient, opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdSweep, guardMutator(getPending, sweepHandler(jfSweeper, opLog)))
	controlServer.RegisterStreaming(daemonipc.CmdParsesAudit, guardMutator(getPending, parsesAuditHandler(db, opLog)))
	controlServer.Register(daemonipc.CmdAttach, daemonipc.AttachHandler(controlServer))
	controlServer.Register(daemonipc.CmdCancel, daemonipc.CancelHandler(controlServer))
	controlServer.Register(daemonipc.CmdListOps, daemonipc.ListOpsHandler(controlServer))
	if err := controlServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start control plane: %w", err)
	}
	defer controlServer.Stop()

	// Start AI enhancement ticker
	if cfg.AI.Enabled && aiMatcher != nil {
		interval := time.Duration(cfg.AI.EnhancementIntervalSeconds) * time.Second
		if interval == 0 {
			interval = 30 * time.Second
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					handler.ProcessPendingAI(ctx)
				case <-ctx.Done():
					return
				}
			}
		}()
		logger.Info("daemon", "AI enhancement ticker started",
			logging.F("interval", interval.String()))
	}

	// Start parse-decision labeler ticker (requires both Jellyfin and DB).
	if jellyfinClient != nil && db != nil {
		fetcher := labeling.JellyfinNameFetcher(func(itemID string) (string, error) {
			item, err := jellyfinClient.GetItem(itemID)
			if err != nil {
				return "", err
			}
			return item.Name, nil
		})
		labeler := labeling.NewRunner(db, fetcher)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("daemon", "Parse-decision labeler panic recovered",
						fmt.Errorf("%v", r))
				}
			}()
			// Short initial delay so the daemon starts up before the first run.
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				return
			}
			ticker := time.NewTicker(15 * time.Minute)
			defer ticker.Stop()
			if err := labeler.RunOnce(); err != nil {
				logger.Warn("daemon", "Parse-decision labeler error", logging.F("error", err.Error()))
			}
			for {
				select {
				case <-ticker.C:
					if err := labeler.RunOnce(); err != nil {
						logger.Warn("daemon", "Parse-decision labeler error", logging.F("error", err.Error()))
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		logger.Info("daemon", "Parse-decision labeler ticker started",
			logging.F("interval", "15m"))
	}

	// Start Jellyfin parse-decision sweeper (requires both Jellyfin and DB).
	if jfSweeper != nil {
		sweeper := jfSweeper
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("daemon", "Jellyfin sweeper panic recovered",
						fmt.Errorf("%v", r))
				}
			}()
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				return
			}
			if err := sweeper.RunOnce(ctx, 24*time.Hour, 7*24*time.Hour); err != nil {
				logger.Warn("daemon", "Jellyfin sweeper error", logging.F("error", err.Error()))
			}
			ticker := time.NewTicker(6 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := sweeper.RunOnce(ctx, 24*time.Hour, 7*24*time.Hour); err != nil {
						logger.Warn("daemon", "Jellyfin sweeper error", logging.F("error", err.Error()))
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		logger.Info("daemon", "Jellyfin parse-decision sweeper started")
	}

	// Housekeeping engine + scheduler: detect cross-volume duplicates,
	// orphan source dirs, stuck syncs, etc., and drain the queued fix
	// tasks under bounded concurrency. See internal/housekeeping.
	if db != nil {
		hkCfg := housekeeping.DefaultConfig()
		hkCfg.TVLibraries = cfg.Libraries.TV
		hkCfg.MovieLibraries = cfg.Libraries.Movies
		hkCfg.WatchDirs = watchPaths
		hkEngine := housekeeping.NewEngine(hkCfg, db, logger)
		hkEngine.SetOpRegistry(controlServer.Registry())

		// Wire optional verifier (Jellyfin RemoteSearch + TMDB direct).
		// Either tier may be unavailable; the verifier degrades gracefully.
		hkVerifier := tmdb.NewVerifier(db, jellyfinClient, cfg.TMDB.APIKey)
		hkEngine.SetVerifier(hkVerifier)

		sched := scheduler.New(db, logger)
		if err := sched.Register(scheduler.Job{
			Name:     "housekeeping.detect",
			Schedule: "@hourly",
			Run: func(ctx context.Context) (string, error) {
				res, err := hkEngine.Detect(ctx)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("enqueued=%d auto_dup=%d cross_volume=%d folder_rename=%d no_year=%d year_mismatch=%d verified_distinct=%d polluted=%d orphan=%d stuck_sync=%d",
					res.Enqueued, res.AutoDupes, res.CrossVolumeDupes, res.FolderRenames, res.NoYearMerges, res.YearMismatches, res.VerifiedDistinct, res.PollutedNames, res.OrphanSources, res.StuckSyncs), nil
			},
		}); err != nil {
			logger.Warn("daemon", "register housekeeping.detect failed", logging.F("error", err.Error()))
		}
		if err := sched.Register(scheduler.Job{
			Name:     "housekeeping.drain",
			Schedule: "@continuous",
			Run: func(ctx context.Context) (string, error) {
				if err := hkEngine.Drain(ctx); err != nil && err != context.Canceled {
					return "", err
				}
				return "drain idle", nil
			},
		}); err != nil {
			logger.Warn("daemon", "register housekeeping.drain failed", logging.F("error", err.Error()))
		}
		// Recovery: prior daemon may have died with rows still in 'running'
		// state (in-memory flag, not persisted). Clear them so the queue
		// can drain and the scheduler can re-fire continuous jobs.
		if n, err := db.RequeueStaleRunningTasks(); err != nil {
			logger.Warn("daemon", "requeue stale running tasks failed", logging.F("error", err.Error()))
		} else if n > 0 {
			logger.Info("daemon", "requeued stale housekeeping tasks", logging.F("count", n))
		}
		if n, err := db.ClearAllRunningJobs(); err != nil {
			logger.Warn("daemon", "clear stale scheduled job running flags failed", logging.F("error", err.Error()))
		} else if n > 0 {
			logger.Info("daemon", "cleared stale scheduled job running flags", logging.F("count", n))
		}

		go sched.Run(ctx)

		controlServer.Register(daemonipc.CmdJobsList, jobsListHandler(db))
		controlServer.Register(daemonipc.CmdJobRun, jobRunHandler(sched, ctx))
		controlServer.Register(daemonipc.CmdJobStop, jobStopHandler(sched))
		controlServer.Register(daemonipc.CmdJobUpdate, jobUpdateHandler(db))
		controlServer.Register(daemonipc.CmdTasksList, tasksListHandler(db))
		controlServer.Register(daemonipc.CmdTaskRetry, taskRetryHandler(db))
		controlServer.Register(daemonipc.CmdTaskCancel, taskCancelHandler(db))
		controlServer.Register(daemonipc.CmdVerifyFlagged, verifyFlaggedHandler(hkEngine))
		controlServer.Register(daemonipc.CmdTaskGet, taskGetHandler(db))
		controlServer.Register(daemonipc.CmdTasksBulk, tasksBulkHandler(db))
		controlServer.Register(daemonipc.CmdTasksPurge, tasksPurgeHandler(db))
		controlServer.Register(daemonipc.CmdTaskVerify, taskVerifyHandler(hkEngine))
		controlServer.Register(daemonipc.CmdTaskGroup, taskGroupHandler(db))
		controlServer.Register(daemonipc.CmdTaskApprove, taskApproveHandler(db))
		logger.Info("daemon", "Scheduler + housekeeping engine started")
	}

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
