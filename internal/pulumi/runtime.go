// Package pulumi provides the Pulumi Automation API runtime for in-process
// infrastructure provisioning as specified in ADR-005. The [Runtime] type
// manages Pulumi stack lifecycle (create, update, destroy) with per-stack
// state isolation.
//
// When a state directory is configured, each stack stores its Pulumi state in
// a deterministic subdirectory: stateDir/<stackName>/. This provides explicit
// per-cluster isolation so that infrastructure operations for one cluster
// cannot accidentally read or mutate the state of another cluster. The mapping
// is stable and reviewable: the same cluster ID always resolves to the same
// state path.
//
// When no state directory is configured (stateDir is empty), each operation
// creates a self-managed temporary workspace that is cleaned up automatically,
// keeping the deployment model simple for local testing.
//
// Idempotency is achieved through Pulumi's declarative model: the program
// describes the desired state and the provider's API handles reconciliation
// (e.g. the Hetzner Cloud provider will not create a duplicate server if one
// with the same name already exists).
package pulumi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	gopulumi "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ProgramFunc is a Pulumi inline program function that defines the desired
// infrastructure state for a stack. Implementations register Pulumi resources
// and export output values via ctx.Export.
type ProgramFunc = gopulumi.RunFunc

// StackConfig holds plaintext key-value configuration applied to a Pulumi
// stack before execution. Keys follow Pulumi's namespaced format (e.g.
// "hcloud:token").
type StackConfig map[string]string

// OutputMap holds the key-value outputs exported by a Pulumi stack program
// after a successful [Runtime.Up] operation.
type OutputMap map[string]any

// UpResult is returned by [Runtime.Up] and contains the stack outputs after
// a successful run.
type UpResult struct {
	// Outputs contains the values exported by the Pulumi program via
	// ctx.Export.
	Outputs OutputMap
}

// Runtime manages Pulumi stack lifecycle in-process using the Automation API
// (ADR-005). It is safe for concurrent use.
//
// When stateDir is non-empty, each stack stores its Pulumi state in a
// dedicated subdirectory (stateDir/<stackName>/) providing deterministic,
// persistent, and isolated state management per cluster. When stateDir is
// empty, each operation uses a transient workspace that is cleaned up
// automatically.
type Runtime struct {
	projectName string
	stateDir    string
}

// New creates a Runtime for the given project name.
//
// projectName must be non-empty. stateDir is the base directory for persistent
// per-stack state storage. When non-empty, each stack stores its state in
// stateDir/<stackName>/, providing explicit per-cluster state isolation. When
// empty, each operation uses a transient workspace that is cleaned up
// automatically.
func New(projectName, stateDir string) (*Runtime, error) {
	if projectName == "" {
		return nil, fmt.Errorf("pulumi: project name must not be empty")
	}
	return &Runtime{projectName: projectName, stateDir: stateDir}, nil
}

// Up creates or selects the named stack in a workspace, applies cfg, runs
// program, and returns the stack outputs.
//
// When the Runtime has a state directory configured, the workspace is
// persistent (stateDir/<stackName>/) and state is preserved between calls.
// When no state directory is configured, a temporary workspace is created and
// removed automatically when the operation completes.
//
// Errors from the Pulumi engine are wrapped with the stack name and operation
// so that callers can surface actionable context to operators.
func (r *Runtime) Up(ctx context.Context, stackName string, program ProgramFunc, cfg StackConfig) (*UpResult, error) {
	workDir, cleanup, err := r.workspaceDir(stackName)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	stack, err := r.upsertStack(ctx, stackName, program, workDir)
	if err != nil {
		return nil, err
	}
	if err := r.applyConfig(ctx, &stack, cfg); err != nil {
		return nil, err
	}
	res, err := stack.Up(ctx, optup.ProgressStreams(os.Stderr), optup.SuppressOutputs())
	if err != nil {
		return nil, fmt.Errorf("pulumi: stack %q up failed: %w", stackName, err)
	}
	return &UpResult{Outputs: outputsFromMap(res.Outputs)}, nil
}

// Destroy selects the named stack in its workspace and destroys all managed
// resources.
//
// When the Runtime has a state directory configured, the persistent workspace
// directory (stateDir/<stackName>/) is used so that Pulumi can locate the
// existing stack state. When no state directory is configured, a temporary
// workspace is used and removed automatically.
//
// Errors are wrapped with the stack name and operation context.
func (r *Runtime) Destroy(ctx context.Context, stackName string, program ProgramFunc) error {
	workDir, cleanup, err := r.workspaceDir(stackName)
	if err != nil {
		return err
	}
	defer cleanup()

	stack, err := r.upsertStack(ctx, stackName, program, workDir)
	if err != nil {
		return err
	}
	if _, err := stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stderr)); err != nil {
		return fmt.Errorf("pulumi: stack %q destroy failed: %w", stackName, err)
	}
	return nil
}

// ProjectName returns the Pulumi project name used by this Runtime.
func (r *Runtime) ProjectName() string {
	return r.projectName
}

// StateDir returns the base state directory configured for this Runtime.
// An empty string indicates that transient workspace behavior is in effect.
func (r *Runtime) StateDir() string {
	return r.stateDir
}

// StackStateDir returns the deterministic persistent state directory for the
// given stack name. The returned path is stateDir/<stackName>, which is stable
// across runs: the same stack name always resolves to the same path.
//
// When the Runtime was created without a state directory, StackStateDir returns
// an empty string, indicating that transient workspace behavior is in effect
// and no persistent state is maintained for the stack.
func (r *Runtime) StackStateDir(stackName string) string {
	if r.stateDir == "" {
		return ""
	}
	return filepath.Join(r.stateDir, stackName)
}

// workspaceDir returns the directory to use as the Pulumi workspace for the
// given stack name, along with a cleanup function to call when done.
//
// When stateDir is set, the directory (stateDir/<stackName>/) is created if
// needed and the cleanup function is a no-op, preserving state between calls.
// When stateDir is empty, a temporary directory is created and the cleanup
// function removes it.
//
// # Accidental state deletion
//
// If the state directory or its parent is deleted at the OS level, the next
// call to [Runtime.Up] or [Runtime.Destroy] will recreate it automatically
// via os.MkdirAll. The infrastructure itself (e.g. Hetzner Cloud servers)
// remains unaffected.
//
// However, if the Pulumi state *files* inside the directory are deleted while
// the corresponding cloud resources still exist, Pulumi treats the stack as
// empty and will attempt to provision new resources on the next [Runtime.Up]
// call. This is the standard Pulumi state-loss scenario: the manager is
// degraded in that it cannot track or destroy the orphaned cloud resources
// through normal Pulumi operations until state is restored (e.g. via
// pulumi state import or by re-creating the resource record). No data or
// currently-running servers are affected; only the management plane loses
// visibility until state is re-established.
func (r *Runtime) workspaceDir(stackName string) (string, func(), error) {
	if r.stateDir != "" {
		dir := filepath.Join(r.stateDir, stackName)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", nil, fmt.Errorf("pulumi: failed to create state directory for stack %q: %w", stackName, err)
		}
		// 0o700 restricts access to the owning process only. Pulumi state
		// files may contain sensitive configuration values; limiting
		// permissions to the owner follows the principle of least privilege.
		return dir, func() {}, nil
	}
	tmpDir, err := os.MkdirTemp("", "lx-container-weaver-pulumi-*")
	if err != nil {
		return "", nil, fmt.Errorf("pulumi: failed to create workspace directory: %w", err)
	}
	return tmpDir, func() { _ = os.RemoveAll(tmpDir) }, nil
}

// upsertStack creates or selects a local inline stack backed by the file-system
// state backend at the given tmpDir.
func (r *Runtime) upsertStack(ctx context.Context, stackName string, program ProgramFunc, tmpDir string) (auto.Stack, error) {
	stateURL := "file://" + filepath.ToSlash(tmpDir)
	proj := workspace.Project{
		Name:    tokens.PackageName(r.projectName),
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		Backend: &workspace.ProjectBackend{URL: stateURL},
	}
	stack, err := auto.UpsertStackInlineSource(ctx, stackName, r.projectName, program,
		auto.Project(proj),
		auto.EnvVars(map[string]string{
			// An empty passphrase enables the passphrase secrets provider
			// without operator interaction.
			"PULUMI_CONFIG_PASSPHRASE": "",
		}),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("pulumi: failed to create or select stack %q: %w", stackName, err)
	}
	return stack, nil
}

// applyConfig sets each entry in cfg as a plaintext config value on the stack.
func (r *Runtime) applyConfig(ctx context.Context, stack *auto.Stack, cfg StackConfig) error {
	for k, v := range cfg {
		if err := stack.SetConfig(ctx, k, auto.ConfigValue{Value: v}); err != nil {
			return fmt.Errorf("pulumi: failed to set config key %q: %w", k, err)
		}
	}
	return nil
}

// outputsFromMap converts the auto.OutputMap to the simpler OutputMap type,
// stripping the secret metadata that is unnecessary for callers.
func outputsFromMap(m auto.OutputMap) OutputMap {
	out := make(OutputMap, len(m))
	for k, v := range m {
		out[k] = v.Value
	}
	return out
}
