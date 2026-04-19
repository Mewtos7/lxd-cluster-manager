package pulumi_test

import (
	"testing"

	pulumiruntime "github.com/Mewtos7/lx-container-weaver/internal/pulumi"
)

func TestNew_EmptyProjectName(t *testing.T) {
	_, err := pulumiruntime.New("")
	if err == nil {
		t.Fatal("New: want error for empty projectName, got nil")
	}
}

func TestNew_ValidProjectName(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("New: expected non-nil runtime")
	}
}

func TestNew_ProjectNameAccessor(t *testing.T) {
	rt, err := pulumiruntime.New("lx-container-weaver")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if rt.ProjectName() != "lx-container-weaver" {
		t.Errorf("ProjectName: got %q, want %q", rt.ProjectName(), "lx-container-weaver")
	}
}
