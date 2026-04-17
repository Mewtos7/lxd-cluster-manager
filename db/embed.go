// Package db exposes the embedded SQL migration files bundled into the binary
// at compile time. Consumers (e.g. cmd/migrate) should pass [Migrations] to
// migration.Runner so that migrations are available without a separate
// on-disk directory at runtime.
package db

import "embed"

// Migrations is an embedded filesystem containing all SQL migration files
// found under db/migrations/. Files are accessible at paths of the form
// "migrations/<filename>.sql".
//
//go:embed migrations/*.sql
var Migrations embed.FS
