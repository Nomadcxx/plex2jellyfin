package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

// Daemon manages the background service
type Daemon struct {
	watcher   *watcher.Watcher
	watchPath string
	enabled   bool
}

// NewDaemon creates a new Daemon instance
func NewDaemon(watcher *watcher.Watcher, watchPath string, enabled bool) *Daemon {
	return &Daemon{
		watcher:   watcher,
		watchPath: watchPath,
		enabled:   enabled,
	}
}

// Start starts the daemon
func (d *Daemon) Start(ctx context.Context) error {
	if !d.enabled {
		log.Println("Daemon is disabled")
		return nil
	}

	log.Printf("Starting jellywatch daemon on: %s\n", d.watchPath)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Start watching in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- d.watcher.Start()
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v\n", sig)
		return d.Stop()

	case err := <-errChan:
		d.watcher.Close()
		return fmt.Errorf("watcher error: %w", err)

	case <-ctx.Done():
		log.Println("Context cancelled")
		return d.Stop()
	}
}

// Stop stops the daemon
func (d *Daemon) Stop() error {
	log.Println("Stopping jellywatch daemon...")
	if err := d.watcher.Close(); err != nil {
		return fmt.Errorf("error closing watcher: %w", err)
	}
	log.Println("Jellywatch daemon stopped")
	return nil
}

// Run runs the daemon with periodic scanning
func (d *Daemon) Run(interval time.Duration) error {
	if !d.enabled {
		log.Println("Daemon is disabled")
		return nil
	}

	log.Printf("Starting jellywatch daemon with %v interval\n", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for {
		select {
		case <-ticker.C:
			log.Println("Periodic scan...")

		case sig := <-sigChan:
			log.Printf("Received signal: %v\n", sig)
			return d.Stop()
		}
	}
}
