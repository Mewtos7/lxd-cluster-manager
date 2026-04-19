package pulumi_test

import (
	"path/filepath"
	"strings"
	"testing"

	pulumiruntime "github.com/Mewtos7/lx-container-weaver/internal/pulumi"
)

func TestNew_EmptyProjectName(t *testing.T) {
	_, err := pulumiruntime.New("", "")
	if err == nil {
		t.Fatal("New: want error for empty projectName, got nil")
	}
}

func TestNew_ValidProjectName(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("New: expected non-nil runtime")
	}
}

func TestNew_ProjectNameAccessor(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt.ProjectName() != "lx-container-weaver" {
		t.Errorf("ProjectName: got %q, want %q", rt.ProjectName(), "lx-container-weaver")
	}
}

// TestNew_StateDirAccessor verifies that StateDir returns the value given at
// construction time.
func TestNew_StateDirAccessor(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "/var/lib/pulumi/state")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt.StateDir() != "/var/lib/pulumi/state" {
		t.Errorf("StateDir: got %q, want %q", rt.StateDir(), "/var/lib/pulumi/state")
	}
}

// TestNew_StateDirEmptyByDefault verifies that StateDir returns an empty string
// when no state directory is provided (transient workspace mode).
func TestNew_StateDirEmptyByDefault(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt.StateDir() != "" {
		t.Errorf("StateDir: want empty string for transient mode, got %q", rt.StateDir())
	}
}

// TestStackStateDir_EmptyWhenNoStateDir verifies that StackStateDir returns an
// empty string when the Runtime was created without a state directory.
func TestStackStateDir_EmptyWhenNoStateDir(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if got := rt.StackStateDir("hetzner-cluster-1"); got != "" {
		t.Errorf("StackStateDir: want empty string for transient mode, got %q", got)
	}
}

// TestStackStateDir_Deterministic verifies that the same stack name always
// resolves to the same state directory path (stable mapping).
func TestStackStateDir_Deterministic(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver", "/state")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	dir1 := rt.StackStateDir("hetzner-cluster-1")
	dir2 := rt.StackStateDir("hetzner-cluster-1")
	if dir1 != dir2 {
		t.Errorf("StackStateDir: want deterministic path, got %q then %q", dir1, dir2)
	}
}

// TestStackStateDir_IsolatedPerCluster verifies that different cluster IDs
// resolve to different, non-overlapping state directories so that one cluster's
// operations cannot interfere with another's.
func TestStackStateDir_IsolatedPerCluster(t *testing.T) {
	baseDir := "/var/lib/pulumi/state"
	rt, err := pulumiruntime.New("lx-container-weaver", baseDir)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	clusterIDs := []string{"cluster-alpha", "cluster-beta", "cluster-gamma"}
	dirs := make(map[string]string, len(clusterIDs))
	for _, id := range clusterIDs {
		stackName := "hetzner-" + id
		dirs[id] = rt.StackStateDir(stackName)
	}

	// Every cluster must have a unique state directory.
	seen := make(map[string]string)
	for id, dir := range dirs {
		if prev, exists := seen[dir]; exists {
			t.Errorf("StackStateDir: cluster %q and %q share state directory %q; isolation violated", id, prev, dir)
		}
		seen[dir] = id
	}

	// Each directory must be a direct child of the base state directory so
	// that no cluster's path is a prefix of another's.
	for id, dir := range dirs {
		parent := filepath.Dir(dir)
		if parent != baseDir {
			t.Errorf("StackStateDir: cluster %q path %q is not a direct child of base dir %q", id, dir, baseDir)
		}
		for otherId, otherDir := range dirs {
			if id == otherId {
				continue
			}
			if strings.HasPrefix(dir+string(filepath.Separator), otherDir+string(filepath.Separator)) {
				t.Errorf("StackStateDir: cluster %q path %q is nested under cluster %q path %q; isolation violated", id, dir, otherId, otherDir)
			}
		}
	}
}

// TestStackStateDir_SubdirOfStateDir verifies that StackStateDir returns a path
// that is a direct child of the configured state directory.
func TestStackStateDir_SubdirOfStateDir(t *testing.T) {
	baseDir := "/var/lib/pulumi/state"
	rt, err := pulumiruntime.New("lx-container-weaver", baseDir)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	stackName := "hetzner-cluster-1"
	dir := rt.StackStateDir(stackName)
	expected := filepath.Join(baseDir, stackName)
	if dir != expected {
		t.Errorf("StackStateDir: want %q, got %q", expected, dir)
	}
}

