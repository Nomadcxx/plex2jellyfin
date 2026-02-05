package main

import (
	"fmt"
	"net/http"

	"github.com/Nomadcxx/jellywatch/internal/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var (
		addr string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the API server",
		Long: `Start the HTTP API server for web UI and external integrations.

The API follows the OpenAPI 3.0 specification defined in api/openapi.yaml.

Examples:
  jellywatch serve                  # Start on default port 8080
  jellywatch serve --addr :9000     # Start on port 9000
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(addr)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Address to listen on")

	return cmd
}

func runServe(addr string) error {
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

	fmt.Printf("Starting Jellywatch API server on %s\n", addr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /api/v1/duplicates    - Get duplicate analysis")
	fmt.Println("  GET  /api/v1/scattered     - Get scattered media analysis")
	fmt.Println("  GET  /api/v1/health        - Health check")
	fmt.Println("  POST /api/v1/scan          - Trigger library scan")

	return http.ListenAndServe(addr, server.Handler())
}
