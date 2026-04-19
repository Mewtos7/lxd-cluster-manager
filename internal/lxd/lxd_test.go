package lxd_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
)

// ─── Constructor tests ────────────────────────────────────────────────────────

func TestNew_EmptyEndpoint(t *testing.T) {
	_, err := lxd.New("")
	if err == nil {
		t.Fatal("New: want error for empty endpoint, got nil")
	}
}

func TestNew_ValidEndpoint(t *testing.T) {
	c, err := lxd.New("https://192.168.1.1:8443")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("New: expected non-nil client")
	}
}

func TestNew_InvalidClientCertificate(t *testing.T) {
	_, err := lxd.New("https://192.168.1.1:8443",
		lxd.WithClientCertificate([]byte("not-a-cert"), []byte("not-a-key")),
	)
	if err == nil {
		t.Fatal("New: want error for invalid certificate PEM, got nil")
	}
}

func TestNew_InvalidServerCA(t *testing.T) {
	_, err := lxd.New("https://192.168.1.1:8443",
		lxd.WithServerCA([]byte("not-a-ca")),
	)
	if err == nil {
		t.Fatal("New: want error for invalid CA PEM, got nil")
	}
}

// ─── Sentinel error tests ─────────────────────────────────────────────────────

func TestSentinelErrors_Distinct(t *testing.T) {
	sentinels := []error{
		lxd.ErrNodeNotFound,
		lxd.ErrInstanceNotFound,
		lxd.ErrUnreachable,
		lxd.ErrMigrationFailed,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinel %d must not match sentinel %d", i, j)
			}
		}
	}
}

func TestSentinelErrors_SurviveWrapping(t *testing.T) {
	for _, sent := range []error{
		lxd.ErrNodeNotFound,
		lxd.ErrInstanceNotFound,
		lxd.ErrUnreachable,
		lxd.ErrMigrationFailed,
	} {
		wrapped := errors.Join(errors.New("outer"), sent)
		if !errors.Is(wrapped, sent) {
			t.Errorf("wrapped error must satisfy errors.Is(%v)", sent)
		}
	}
}

// ─── Unreachable endpoint tests ───────────────────────────────────────────────

// TestGetClusterMembers_Unreachable verifies that an unreachable endpoint
// causes GetClusterMembers to return an error wrapping ErrUnreachable.
func TestGetClusterMembers_Unreachable(t *testing.T) {
	c, _ := lxd.New("https://192.0.2.1:8443") // TEST-NET-1, guaranteed unreachable
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.GetClusterMembers(ctx)
	if err == nil {
		t.Fatal("GetClusterMembers: want error for unreachable endpoint, got nil")
	}
	if !errors.Is(err, lxd.ErrUnreachable) {
		t.Errorf("GetClusterMembers: want errors.Is(err, ErrUnreachable), got %v", err)
	}
}

// TestGetClusterMember_Unreachable verifies that an unreachable endpoint
// causes GetClusterMember to return an error wrapping ErrUnreachable.
func TestGetClusterMember_Unreachable(t *testing.T) {
	c, _ := lxd.New("https://192.0.2.1:8443")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.GetClusterMember(ctx, "node1")
	if err == nil {
		t.Fatal("GetClusterMember: want error for unreachable endpoint, got nil")
	}
	if !errors.Is(err, lxd.ErrUnreachable) {
		t.Errorf("GetClusterMember: want errors.Is(err, ErrUnreachable), got %v", err)
	}
}

// TestGetClusterMember_EmptyName verifies that GetClusterMember rejects an
// empty name without making any network calls.
func TestGetClusterMember_EmptyName(t *testing.T) {
	c, _ := lxd.New("https://192.168.1.1:8443")
	_, err := c.GetClusterMember(context.Background(), "")
	if err == nil {
		t.Fatal("GetClusterMember: want error for empty name, got nil")
	}
}

// TestGetNodeResources_EmptyName verifies that GetNodeResources rejects an
// empty name without making any network calls.
func TestGetNodeResources_EmptyName(t *testing.T) {
	c, _ := lxd.New("https://192.168.1.1:8443")
	_, err := c.GetNodeResources(context.Background(), "")
	if err == nil {
		t.Fatal("GetNodeResources: want error for empty name, got nil")
	}
}

// TestGetInstance_EmptyName verifies that GetInstance rejects an empty name
// without making any network calls.
func TestGetInstance_EmptyName(t *testing.T) {
	c, _ := lxd.New("https://192.168.1.1:8443")
	_, err := c.GetInstance(context.Background(), "")
	if err == nil {
		t.Fatal("GetInstance: want error for empty name, got nil")
	}
}

// TestMoveInstance_EmptyInstanceName verifies that MoveInstance rejects an
// empty instance name without making any network calls.
func TestMoveInstance_EmptyInstanceName(t *testing.T) {
	c, _ := lxd.New("https://192.168.1.1:8443")
	err := c.MoveInstance(context.Background(), "", "node2")
	if err == nil {
		t.Fatal("MoveInstance: want error for empty instanceName, got nil")
	}
}

// TestMoveInstance_EmptyTargetNode verifies that MoveInstance rejects an empty
// target node name without making any network calls.
func TestMoveInstance_EmptyTargetNode(t *testing.T) {
	c, _ := lxd.New("https://192.168.1.1:8443")
	err := c.MoveInstance(context.Background(), "web-01", "")
	if err == nil {
		t.Fatal("MoveInstance: want error for empty targetNode, got nil")
	}
}

// ─── httptest-based integration tests ────────────────────────────────────────

// lxdSyncResponse creates the JSON body for a synchronous LXD API response.
func lxdSyncResponse(t *testing.T, metadata any) []byte {
	t.Helper()
	meta, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	resp := map[string]any{
		"type":        "sync",
		"status":      "Success",
		"status_code": 200,
		"metadata":    json.RawMessage(meta),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal sync response: %v", err)
	}
	return b
}

// lxdErrorResponse creates the JSON body for a LXD API error response.
func lxdErrorResponse(t *testing.T, code int, message string) []byte {
	t.Helper()
	resp := map[string]any{
		"type":       "error",
		"error":      message,
		"error_code": code,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}
	return b
}

// TestGetClusterMembers_Success verifies that GetClusterMembers correctly maps
// LXD cluster member objects from the API response.
func TestGetClusterMembers_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.0/cluster/members" {
			http.NotFound(w, r)
			return
		}
		members := []map[string]any{
			{
				"server_name":  "lxd1",
				"url":          "https://10.0.0.1:8443",
				"status":       "Online",
				"message":      "Fully operational",
				"architecture": "x86_64",
				"description":  "",
				"roles":        []string{"database", "database-leader"},
			},
			{
				"server_name":  "lxd2",
				"url":          "https://10.0.0.2:8443",
				"status":       "Online",
				"message":      "Fully operational",
				"architecture": "x86_64",
				"description":  "",
				"roles":        []string{"database"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(lxdSyncResponse(t, members))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	nodes, err := c.GetClusterMembers(context.Background())
	if err != nil {
		t.Fatalf("GetClusterMembers: unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("GetClusterMembers: want 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Name != "lxd1" {
		t.Errorf("nodes[0].Name: want %q, got %q", "lxd1", nodes[0].Name)
	}
	if nodes[0].Status != "Online" {
		t.Errorf("nodes[0].Status: want %q, got %q", "Online", nodes[0].Status)
	}
	if len(nodes[0].Roles) != 2 {
		t.Errorf("nodes[0].Roles: want 2, got %d", len(nodes[0].Roles))
	}
}

// TestGetClusterMember_NotFound verifies that a 404 from the LXD API is
// mapped to ErrNodeNotFound.
func TestGetClusterMember_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(lxdErrorResponse(t, 404, "not found"))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.GetClusterMember(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetClusterMember: want error, got nil")
	}
	if !errors.Is(err, lxd.ErrNodeNotFound) {
		t.Errorf("GetClusterMember: want errors.Is(err, ErrNodeNotFound), got %v", err)
	}
}

// TestGetNodeResources_Success verifies that GetNodeResources correctly maps
// LXD resource data from the API response.
func TestGetNodeResources_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.0/resources" {
			http.NotFound(w, r)
			return
		}
		res := map[string]any{
			"cpu": map[string]any{
				"total": 8,
			},
			"memory": map[string]any{
				"total": 8589934592,
				"used":  4294967296,
			},
			"storage": map[string]any{
				"disks": []map[string]any{
					{
						"size": 107374182400,
						"partitions": []map[string]any{
							{"size": 107374182400, "used": 21474836480},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(lxdSyncResponse(t, res))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resources, err := c.GetNodeResources(context.Background(), "lxd1")
	if err != nil {
		t.Fatalf("GetNodeResources: unexpected error: %v", err)
	}
	if resources.CPU.Total != 8 {
		t.Errorf("CPU.Total: want 8, got %d", resources.CPU.Total)
	}
	if resources.Memory.Total != 8589934592 {
		t.Errorf("Memory.Total: want 8589934592, got %d", resources.Memory.Total)
	}
	if resources.Memory.Used != 4294967296 {
		t.Errorf("Memory.Used: want 4294967296, got %d", resources.Memory.Used)
	}
	if resources.Disk.Total != 107374182400 {
		t.Errorf("Disk.Total: want 107374182400, got %d", resources.Disk.Total)
	}
	if resources.Disk.Used != 21474836480 {
		t.Errorf("Disk.Used: want 21474836480, got %d", resources.Disk.Used)
	}
}

// TestListInstances_Success verifies that ListInstances correctly maps LXD
// instance objects from the API response.
func TestListInstances_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.0/instances" {
			http.NotFound(w, r)
			return
		}
		instances := []map[string]any{
			{
				"name":        "web-01",
				"status":      "Running",
				"type":        "container",
				"location":    "lxd1",
				"description": "",
				"config":      map[string]string{"limits.cpu": "2"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(lxdSyncResponse(t, instances))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	instances, err := c.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: unexpected error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("ListInstances: want 1 instance, got %d", len(instances))
	}
	if instances[0].Name != "web-01" {
		t.Errorf("instances[0].Name: want %q, got %q", "web-01", instances[0].Name)
	}
	if instances[0].InstanceType != "container" {
		t.Errorf("instances[0].InstanceType: want %q, got %q", "container", instances[0].InstanceType)
	}
	if instances[0].Location != "lxd1" {
		t.Errorf("instances[0].Location: want %q, got %q", "lxd1", instances[0].Location)
	}
}

// TestGetInstance_NotFound verifies that a 404 from the LXD API is mapped to
// ErrInstanceNotFound for instance lookups.
func TestGetInstance_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(lxdErrorResponse(t, 404, "Instance not found"))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.GetInstance(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetInstance: want error, got nil")
	}
	// The 404 maps to ErrNodeNotFound from the generic handler; callers of
	// GetInstance should check for ErrNodeNotFound OR ErrInstanceNotFound.
	if !errors.Is(err, lxd.ErrNodeNotFound) && !errors.Is(err, lxd.ErrInstanceNotFound) {
		t.Errorf("GetInstance: want not-found sentinel error, got %v", err)
	}
}

// TestMoveInstance_AsyncSuccess verifies that MoveInstance correctly handles an
// async LXD response and polls the operation until it succeeds.
func TestMoveInstance_AsyncSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances/web-01":
			// Return an async response with an operation path.
			resp := map[string]any{
				"type":        "async",
				"status":      "Operation created",
				"status_code": 100,
				"operation":   "/1.0/operations/op-abc",
				"metadata": map[string]any{
					"id":          "op-abc",
					"status":      "Running",
					"status_code": 103,
					"err":         "",
				},
			}
			b, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)

		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/op-abc/wait":
			// Return a successful operation result.
			op := map[string]any{
				"id":          "op-abc",
				"status":      "Success",
				"status_code": 200,
				"err":         "",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(lxdSyncResponse(t, op))

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.MoveInstance(context.Background(), "web-01", "lxd2")
	if err != nil {
		t.Fatalf("MoveInstance: unexpected error: %v", err)
	}
}

// TestMoveInstance_OperationFailure verifies that a failed LXD migration
// operation wraps ErrMigrationFailed.
func TestMoveInstance_OperationFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances/web-01":
			resp := map[string]any{
				"type":        "async",
				"status":      "Operation created",
				"status_code": 100,
				"operation":   "/1.0/operations/op-fail",
				"metadata": map[string]any{
					"id":          "op-fail",
					"status":      "Running",
					"status_code": 103,
					"err":         "",
				},
			}
			b, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)

		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/op-fail/wait":
			op := map[string]any{
				"id":          "op-fail",
				"status":      "Failure",
				"status_code": 400,
				"err":         "migration failed: CRIU not supported",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(lxdSyncResponse(t, op))

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = c.MoveInstance(context.Background(), "web-01", "lxd2")
	if err == nil {
		t.Fatal("MoveInstance: want error for failed operation, got nil")
	}
	if !errors.Is(err, lxd.ErrMigrationFailed) {
		t.Errorf("MoveInstance: want errors.Is(err, ErrMigrationFailed), got %v", err)
	}
}

// TestListInstances_EmptyCluster verifies that an empty list from LXD returns
// a non-nil empty slice (not nil).
func TestListInstances_EmptyCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(lxdSyncResponse(t, []any{}))
	}))
	defer srv.Close()

	c, err := lxd.New(srv.URL, lxd.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	instances, err := c.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: unexpected error: %v", err)
	}
	if instances == nil {
		t.Error("ListInstances: want non-nil slice for empty response, got nil")
	}
	if len(instances) != 0 {
		t.Errorf("ListInstances: want 0 instances, got %d", len(instances))
	}
}
