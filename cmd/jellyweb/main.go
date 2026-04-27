package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyweb/daemonctl"
	"github.com/Nomadcxx/jellywatch/internal/paths"
	"github.com/spf13/cobra"
)

var (
	port string
	host string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "jellyweb",
		Short: "JellyWatch Web UI Server",
		Long: `JellyWatch Web UI Server provides a web interface for managing
your media library organization. It serves the embedded Next.js UI
and exposes a REST API for media operations.`,
		RunE: runServer,
	}

	rootCmd.Flags().StringVarP(&port, "port", "p", "5522", "Port to listen on")
	rootCmd.Flags().StringVarP(&host, "host", "H", "0.0.0.0", "Host to bind to")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	configPath, err := paths.ConfigPath()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	dbPath, err := paths.DatabasePath()
	if err != nil {
		return fmt.Errorf("failed to resolve database path: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	server := api.NewServer(db, cfg)
	if binary, err := os.Executable(); err == nil {
		logPath := ""
		if dir, derr := paths.JellyWatchDir(); derr == nil {
			logPath = dir + "/jellywatchd.log"
		}
		// Replace jellyweb basename with jellywatchd so the launcher invokes
		// the daemon binary, not this web server, when falling back to direct exec.
		if idx := strings.LastIndex(binary, "/"); idx >= 0 {
			binary = binary[:idx+1] + "jellywatchd"
		} else {
			binary = "jellywatchd"
		}
		server.SetLauncher(daemonctl.New(binary, logPath))
	}
	handler := server.Handler()

	addr := net.JoinHostPort(host, port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Printf("🌐 JellyWatch Web UI starting on http://%s\n", addr)
		fmt.Printf("📁 Config: %s\n", configPath)
		fmt.Printf("🗄️  Database: %s\n", dbPath)
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			fmt.Printf("👤 Runtime user: %s (SUDO_USER=%s)\n", paths.ActualUser(), sudoUser)
		} else {
			fmt.Printf("👤 Runtime user: %s\n", paths.ActualUser())
		}
		if cfg.Password != "" {
			fmt.Println("🔐 Authentication enabled - login required")
		} else {
			fmt.Println("⚠️  No password set - authentication disabled")
		}
		fmt.Println()

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stop
	fmt.Println("\n🛑 Shutting down web server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	server.Close()

	fmt.Println("✅ Web server stopped gracefully")
	return nil
}
