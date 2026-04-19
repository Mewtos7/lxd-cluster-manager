package bootstrap_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd/fake"
)

// testCert is a placeholder PEM certificate reused across test functions.
const testCert = "-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----"

// ─── stub ReadinessChecker ────────────────────────────────────────────────────

// stubCheck is a ReadinessChecker whose behaviour is controlled by the test.
type stubCheck struct {
	name string
	err  error // nil = pass, non-nil = fail with this error
}

func (s *stubCheck) Name() string                  { return s.name }
func (s *stubCheck) Check(_ context.Context) error { return s.err }

// ─── helpers ─────────────────────────────────────────────────────────────────

// newSuccessfulBootstrap returns a seed + joiner fake pair whose state allows
// Bootstrap to complete without error, and a minimalConfig that refers to the
// same node names.
func newSuccessfulBootstrap() (seed, joiner *fake.Fake, cfg bootstrap.Config) {
	seed = fake.New()
	seed.SetClusterCertificate(testCert)
	seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	seed.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})

	joiner = fake.New()

	cfg = bootstrap.Config{
		ClusterName:   "test-cluster",
		TrustToken:    "s3cr3t",
		StorageDriver: "dir",
		StoragePool:   "default",
		SeedNode: bootstrap.NodeConfig{
			Name:          "lxd1",
			ListenAddress: "10.0.0.1:8443",
		},
		JoinerNode: bootstrap.NodeConfig{
			Name:          "lxd2",
			ListenAddress: "10.0.0.2:8443",
		},
	}
	return
}

// ─── Workflow tests ───────────────────────────────────────────────────────────

// TestWorkflow_Success verifies that when all readiness checks pass and
// bootstrap succeeds, Result.Ready is true and Err is nil.
func TestWorkflow_Success(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	check := &stubCheck{name: "seed-lxd", err: nil}
	wf := bootstrap.NewWorkflow(b, check)

	result := wf.Run(context.Background(), "lxd1", cfg)

	if !result.Ready {
		t.Fatalf("Run: want Ready=true, got false (err: %v)", result.Err)
	}
	if result.Err != nil {
		t.Errorf("Run: want Err=nil, got %v", result.Err)
	}
	if result.FailedStep != "" {
		t.Errorf("Run: want FailedStep empty, got %q", result.FailedStep)
	}
	if result.NodeName != "lxd1" {
		t.Errorf("Run: want NodeName %q, got %q", "lxd1", result.NodeName)
	}
}

// TestWorkflow_NoChecks verifies that the workflow runs bootstrap even when no
// readiness checks are registered.
func TestWorkflow_NoChecks(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	wf := bootstrap.NewWorkflow(b) // zero checks

	result := wf.Run(context.Background(), "lxd1", cfg)

	if !result.Ready {
		t.Fatalf("Run (no checks): want Ready=true, got false (err: %v)", result.Err)
	}
}

// TestWorkflow_ReadinessCheckFailure verifies that when a readiness check
// fails, bootstrap is not attempted and Result.Ready is false.
func TestWorkflow_ReadinessCheckFailure(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	checkErr := errors.New("endpoint unreachable")
	check := &stubCheck{name: "seed-lxd", err: checkErr}
	wf := bootstrap.NewWorkflow(b, check)

	result := wf.Run(context.Background(), "lxd1", cfg)

	if result.Ready {
		t.Fatal("Run: want Ready=false when readiness check fails, got true")
	}
	if result.Err == nil {
		t.Fatal("Run: want non-nil Err when readiness check fails")
	}
	if !errors.Is(result.Err, checkErr) {
		t.Errorf("Run: want Err to wrap %v, got %v", checkErr, result.Err)
	}
	wantStep := "readiness:seed-lxd"
	if result.FailedStep != wantStep {
		t.Errorf("Run: want FailedStep %q, got %q", wantStep, result.FailedStep)
	}

	// Bootstrap must not have been called.
	if len(seed.InitCalls) != 0 {
		t.Errorf("InitCalls: want 0 when readiness check fails, got %d", len(seed.InitCalls))
	}
	if len(joiner.JoinCalls) != 0 {
		t.Errorf("JoinCalls: want 0 when readiness check fails, got %d", len(joiner.JoinCalls))
	}
}

// TestWorkflow_FirstReadinessCheckFails verifies that when there are multiple
// checks and the first fails, no subsequent checks or bootstrap are run.
func TestWorkflow_FirstReadinessCheckFails(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	firstErr := errors.New("node not yet reachable")
	firstCheck := &stubCheck{name: "first", err: firstErr}
	secondCheck := &stubCheck{name: "second", err: nil} // would pass

	wf := bootstrap.NewWorkflow(b, firstCheck, secondCheck)

	result := wf.Run(context.Background(), "lxd1", cfg)

	if result.Ready {
		t.Fatal("Run: want Ready=false when first check fails")
	}
	if !strings.HasPrefix(result.FailedStep, "readiness:first") {
		t.Errorf("FailedStep: want prefix %q, got %q", "readiness:first", result.FailedStep)
	}

	// Neither bootstrap call should have been made.
	if len(seed.InitCalls) != 0 || len(joiner.JoinCalls) != 0 {
		t.Error("Bootstrap must not be called when the first readiness check fails")
	}
}

// TestWorkflow_AllReadinessChecksMustPass verifies that all checks are
// evaluated in order and all must pass for bootstrap to run.
func TestWorkflow_AllReadinessChecksMustPass(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	secondErr := errors.New("joiner not ready")
	firstCheck := &stubCheck{name: "seed-check", err: nil}          // passes
	secondCheck := &stubCheck{name: "joiner-check", err: secondErr} // fails

	wf := bootstrap.NewWorkflow(b, firstCheck, secondCheck)

	result := wf.Run(context.Background(), "lxd1", cfg)

	if result.Ready {
		t.Fatal("Run: want Ready=false when second check fails")
	}
	wantStep := "readiness:joiner-check"
	if result.FailedStep != wantStep {
		t.Errorf("FailedStep: want %q, got %q", wantStep, result.FailedStep)
	}
	if len(seed.InitCalls) != 0 || len(joiner.JoinCalls) != 0 {
		t.Error("Bootstrap must not be called when a readiness check fails")
	}
}

// TestWorkflow_BootstrapFailure verifies that when readiness checks pass but
// bootstrap fails, Result.Ready is false and FailedStep is "bootstrap".
func TestWorkflow_BootstrapFailure(t *testing.T) {
	seed := fake.New()
	seed.SetClusterCertificate(testCert)
	seed.InitError = errors.New("disk full")

	joiner := fake.New()

	cfg := bootstrap.Config{
		ClusterName:   "test-cluster",
		TrustToken:    "s3cr3t",
		StorageDriver: "dir",
		StoragePool:   "default",
		SeedNode:      bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
		JoinerNode:    bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
	}

	b := bootstrap.New(seed, joiner)
	check := &stubCheck{name: "seed-lxd", err: nil}
	wf := bootstrap.NewWorkflow(b, check)

	result := wf.Run(context.Background(), "lxd1", cfg)

	if result.Ready {
		t.Fatal("Run: want Ready=false when bootstrap fails, got true")
	}
	if result.Err == nil {
		t.Fatal("Run: want non-nil Err when bootstrap fails")
	}
	if result.FailedStep != "bootstrap" {
		t.Errorf("FailedStep: want %q, got %q", "bootstrap", result.FailedStep)
	}
}

// TestWorkflow_ResultNodeName verifies that the NodeName field in the result
// always matches the nodeName argument passed to Run.
func TestWorkflow_ResultNodeName(t *testing.T) {
	seed, joiner, cfg := newSuccessfulBootstrap()
	b := bootstrap.New(seed, joiner)

	wf := bootstrap.NewWorkflow(b)

	result := wf.Run(context.Background(), "my-node-42", cfg)

	if result.NodeName != "my-node-42" {
		t.Errorf("NodeName: want %q, got %q", "my-node-42", result.NodeName)
	}
}

// TestWorkflow_FailureReadyIsFalse verifies that any failure — whether from a
// readiness check or from bootstrap — always produces Ready=false, ensuring
// the node is never silently treated as available capacity.
func TestWorkflow_FailureReadyIsFalse(t *testing.T) {
	tests := []struct {
		name         string
		checkErr     error
		bootstrapErr bool
	}{
		{
			name:     "readiness check fails",
			checkErr: errors.New("not reachable"),
		},
		{
			name:         "bootstrap fails",
			bootstrapErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seed := fake.New()
			seed.SetClusterCertificate("cert")
			seed.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
			if tc.bootstrapErr {
				seed.InitError = errors.New("bootstrap failure")
			}

			joiner := fake.New()

			cfg := bootstrap.Config{
				ClusterName:   "c",
				TrustToken:    "t",
				StorageDriver: "dir",
				StoragePool:   "default",
				SeedNode:      bootstrap.NodeConfig{Name: "lxd1", ListenAddress: "10.0.0.1:8443"},
				JoinerNode:    bootstrap.NodeConfig{Name: "lxd2", ListenAddress: "10.0.0.2:8443"},
			}

			b := bootstrap.New(seed, joiner)
			check := &stubCheck{name: "check", err: tc.checkErr}
			wf := bootstrap.NewWorkflow(b, check)

			result := wf.Run(context.Background(), "lxd1", cfg)

			if result.Ready {
				t.Errorf("%s: Ready must be false on failure", tc.name)
			}
			if result.Err == nil {
				t.Errorf("%s: Err must be non-nil on failure", tc.name)
			}
		})
	}
}

// TestWorkflow_LXDReadinessCheck_Pass verifies that LXDReadinessCheck passes
// when the client can return cluster status without error.
func TestWorkflow_LXDReadinessCheck_Pass(t *testing.T) {
	f := fake.New() // returns a valid ClusterStatus by default

	check := bootstrap.NewLXDReadinessCheck("seed", f)

	if err := check.Check(context.Background()); err != nil {
		t.Fatalf("Check: unexpected error: %v", err)
	}
	if check.Name() != "seed" {
		t.Errorf("Name: want %q, got %q", "seed", check.Name())
	}
}

// TestWorkflow_LXDReadinessCheck_Fail verifies that LXDReadinessCheck fails
// when the client returns an error from GetClusterStatus.
func TestWorkflow_LXDReadinessCheck_Fail(t *testing.T) {
	// Simulate an unreachable node by making the cluster status return an error.
	// The fake does not have a built-in error injection for GetClusterStatus,
	// so we use a lightweight wrapper to simulate the failure.
	client := &alwaysUnreachableClient{}

	check := bootstrap.NewLXDReadinessCheck("seed", client)

	if err := check.Check(context.Background()); err == nil {
		t.Fatal("Check: want error when endpoint is unreachable, got nil")
	}
}

// alwaysUnreachableClient is a minimal lxd.Client stub that returns
// ErrUnreachable from GetClusterStatus and panics on any other call.
type alwaysUnreachableClient struct{}

func (a *alwaysUnreachableClient) GetClusterStatus(_ context.Context) (*lxd.ClusterStatus, error) {
	return nil, lxd.ErrUnreachable
}

func (a *alwaysUnreachableClient) GetClusterMembers(_ context.Context) ([]lxd.NodeInfo, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) GetClusterMember(_ context.Context, _ string) (*lxd.NodeInfo, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) GetNodeResources(_ context.Context, _ string) (*lxd.NodeResources, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) ListInstances(_ context.Context) ([]lxd.InstanceInfo, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) GetInstance(_ context.Context, _ string) (*lxd.InstanceInfo, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) MoveInstance(_ context.Context, _, _ string) error {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) GetClusterCertificate(_ context.Context) (string, error) {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) InitCluster(_ context.Context, _ lxd.ClusterInitConfig) error {
	panic("unexpected call")
}
func (a *alwaysUnreachableClient) JoinCluster(_ context.Context, _ lxd.ClusterJoinConfig) error {
	panic("unexpected call")
}
