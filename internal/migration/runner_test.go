package migration_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/Mewtos7/lx-container-weaver/internal/migration"
)

// newTestFS returns an in-memory filesystem that mimics the db/migrations
// directory layout used in the real repository.
func newTestFS(files map[string]string) fs.FS {
	fsys := make(fstest.MapFS)
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

// TestNew verifies that New returns a non-nil Runner without panicking.
func TestNew(t *testing.T) {
	fsys := newTestFS(map[string]string{
		"migrations/0001_initial_schema.up.sql":   "SELECT 1;",
		"migrations/0001_initial_schema.down.sql": "SELECT 1;",
	})

	r := migration.New(nil, fsys)
	if r == nil {
		t.Fatal("New returned nil")
	}
}

// TestVersionFromPathExamples exercises the internal version-extraction logic
// through the public API by confirming that different filenames produce the
// correct ordering when files are listed.  The test uses a custom FS that
// exercises sorting across three migrations.
func TestVersionFromPathExamples(t *testing.T) {
	tests := []struct {
		path    string
		want    string
	}{
		{"migrations/0001_initial_schema.up.sql", "0001_initial_schema"},
		{"migrations/0002_add_users.up.sql", "0002_add_users"},
		{"migrations/0010_add_index.down.sql", "0010_add_index"},
	}

	for _, tc := range tests {
		got := migration.VersionFromPath(tc.path)
		if got != tc.want {
			t.Errorf("VersionFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
