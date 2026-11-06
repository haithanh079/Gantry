package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go2engle/gantry/internal/api/handlers"
	"github.com/go2engle/gantry/internal/config"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	"github.com/spf13/cobra"
)

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate catalog data into newer landscape kinds",
	}
	landscape := &cobra.Command{
		Use:   "landscape",
		Short: "Create Deployment entities from deployedIn (dry-run unless --apply)",
		RunE:  runMigrateLandscape,
	}
	landscape.Flags().Bool("apply", false, "Write the planned entities. Without this flag the command only prints the plan.")
	landscape.Flags().String("config", "", "Path to config file (default: gantry.yaml)")
	landscape.Flags().String("db", "", "Database connection string")
	cmd.AddCommand(landscape)
	return cmd
}

func runMigrateLandscape(cmd *cobra.Command, _ []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "gantry.yaml"
	}
	cfg := config.LoadWithFile(configPath)
	if dbConn, _ := cmd.Flags().GetString("db"); dbConn != "" {
		cfg.DBDSN = dbConn
		if len(dbConn) > 10 && (dbConn[:11] == "postgres://" || dbConn[:13] == "postgresql://") {
			cfg.DBType = "postgres"
		} else {
			cfg.DBType = "sqlite"
		}
	}
	database, err := db.New(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	validator, err := entity.NewSchemaValidator("")
	if err != nil {
		return fmt.Errorf("loading schemas: %w", err)
	}
	h := &handlers.Handlers{DB: database, Events: events.New(), Validator: validator}
	apply, _ := cmd.Flags().GetBool("apply")
	result, err := h.ApplyLandscapeMigration(context.Background(), apply)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return err
	}
	if !apply {
		fmt.Fprintf(os.Stderr, "dry-run: %d deployments would be created. Re-run with --apply to write them.\n", result.Deployments)
	}
	return nil
}
