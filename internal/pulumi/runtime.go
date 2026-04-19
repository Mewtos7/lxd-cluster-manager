// Package pulumi provides the Pulumi Automation API runtime for in-process
// infrastructure provisioning as specified in ADR-005. The [Runtime] type
// manages Pulumi stack lifecycle (create, update, destroy) in a transient
// workspace: no persistent state backend is required or configured by the
// operator. Each operation creates a self-managed temporary workspace that is
// cleaned up automatically, keeping the deployment model simple and stateless
// from the operator's perspective.
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
// Each operation creates a self-managed temporary workspace directory that is
// removed after the operation completes. No persistent state backend needs to
// be configured by the operator. Idempotency is achieved through Pulumi's
// declarative model combined with the provider's idempotent API.
type Runtime struct {
	projectName string
}

// New creates a Runtime for the given project name.
//
// projectName must be non-empty. No filesystem paths or backends need to be
// configured; the runtime manages transient workspaces automatically.
func New(projectName string) (*Runtime, error) {
	if projectName == "" {
		return nil, fmt.Errorf("pulumi: project name must not be empty")
	}
	return &Runtime{projectName: projectName}, nil
}

// Up creates or selects the named stack in a transient workspace, applies cfg,
// runs program, and returns the stack outputs. The temporary workspace is
// removed automatically when the operation completes.
//
// Errors from the Pulumi engine are wrapped with the stack name and operation
// so that callers can surface actionable context to operators.
func (r *Runtime) Up(ctx context.Context, stackName string, program ProgramFunc, cfg StackConfig) (*UpResult, error) {
	tmpDir, err := os.MkdirTemp("", "lx-container-weaver-pulumi-*")
	if err != nil {
		return nil, fmt.Errorf("pulumi: failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	stack, err := r.upsertStack(ctx, stackName, program, tmpDir)
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

// Destroy creates a transient workspace for the named stack and destroys all
// its managed resources. The temporary workspace is removed automatically.
//
// Errors are wrapped with the stack name and operation context.
func (r *Runtime) Destroy(ctx context.Context, stackName string, program ProgramFunc) error {
	tmpDir, err := os.MkdirTemp("", "lx-container-weaver-pulumi-*")
	if err != nil {
		return fmt.Errorf("pulumi: failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	stack, err := r.upsertStack(ctx, stackName, program, tmpDir)
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
