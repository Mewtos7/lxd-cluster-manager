// Command migrate applies or rolls back database migrations for the
// LX Container Weaver manager service.
//
// Usage:
//
//	migrate up    — apply all pending forward migrations
//	migrate down  — roll back the most recently applied migration
//	migrate status — list applied migrations and their timestamps
//
// The target database is specified via the DATABASE_URL environment variable,
// which must be a valid PostgreSQL connection string, for example:
//
//	DATABASE_URL=postgres://weaver:secret@localhost:5432/weaver?sslmode=disable
//
// Migration files are embedded at compile time from db/migrations/.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	weavedb "github.com/Mewtos7/lx-container-weaver/db"
	"github.com/Mewtos7/lx-container-weaver/internal/migration"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL environment variable is required")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		logger.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logger.Error("database connection check failed", "error", err)
		os.Exit(1)
	}

	runner := migration.New(db, weavedb.Migrations)
	ctx := context.Background()

	switch command {
	case "up":
		count, err := runner.Up(ctx)
		if err != nil {
			logger.Error("migration up failed", "error", err)
			os.Exit(1)
		}
		if count == 0 {
			logger.Info("no pending migrations — database schema is up to date")
		} else {
			logger.Info("migrations applied successfully", "count", count)
		}

	case "down":
		count, err := runner.Down(ctx)
		if err != nil {
			logger.Error("migration down failed", "error", err)
			os.Exit(1)
		}
		if count == 0 {
			logger.Info("no applied migrations to roll back")
		} else {
			logger.Info("migration rolled back successfully")
		}

	case "status":
		applied, err := runner.Status(ctx)
		if err != nil {
			logger.Error("failed to retrieve migration status", "error", err)
			os.Exit(1)
		}
		if len(applied) == 0 {
			fmt.Println("No migrations applied.")
			return
		}
		fmt.Printf("%-40s  %s\n", "VERSION", "APPLIED AT")
		fmt.Printf("%-40s  %s\n", strings.Repeat("-", 40), strings.Repeat("-", 25))
		for _, m := range applied {
			fmt.Printf("%-40s  %s\n", m.Version, m.AppliedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
		}

	default:
		logger.Error("unknown command", "command", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: migrate <command>

Commands:
  up      Apply all pending forward migrations
  down    Roll back the most recently applied migration
  status  Show which migrations have been applied

Environment variables:
  DATABASE_URL  PostgreSQL connection string (required)
               Example: postgres://user:pass@localhost:5432/dbname?sslmode=disable`)
}
