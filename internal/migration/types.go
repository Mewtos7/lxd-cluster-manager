package migration

import "time"

// AppliedMigration is a record of a migration version that has been applied to
// the database.
type AppliedMigration struct {
	Version   string
	AppliedAt time.Time
}
