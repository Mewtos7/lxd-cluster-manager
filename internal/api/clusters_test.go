package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/api"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"golang.org/x/crypto/bcrypt"
)

// newClusterServer returns a Server wired with an in-memory ClusterStore and a
// single bcrypt-hashed API key. The raw key value is also returned so callers
// can set the Authorization header on protected requests.
func newClusterServer(t *testing.T) (*api.Server, string) {
	t.Helper()
	const rawKey = "test-cluster-key"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	store := memory.NewClusterStore()
	srv := api.New(":0", slog.Default(), []string{string(hash)},
		api.WithClusterRepository(store))
	return srv, rawKey
}

// authHeader returns a Bearer authorization header value for key.
func authHeader(key string) string {
	return "Bearer " + key
}

// jsonBody encodes v as JSON and returns an *bytes.Reader suitable for use as
// an HTTP request body.
func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewReader(b)
}

// decodeCluster decodes a *model.Cluster from the response body.
func decodeCluster(t *testing.T, rec *httptest.ResponseRecorder) *model.Cluster {
	t.Helper()
	var c model.Cluster
	if err := json.NewDecoder(rec.Body).Decode(&c); err != nil {
		t.Fatalf("decode cluster: %v", err)
	}
	return &c
}

// decodeClusterList decodes the paginated cluster list response.
func decodeClusterList(t *testing.T, rec *httptest.ResponseRecorder) (items []*model.Cluster, total int) {
	t.Helper()
	var resp struct {
		Items []*model.Cluster `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode cluster list: %v", err)
	}
	return resp.Items, resp.Total
}

// ─── List Clusters ────────────────────────────────────────────────────────────

func TestListClusters_Empty(t *testing.T) {
	srv, key := newClusterServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/clusters", nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	items, total := decodeClusterList(t, rec)
	if total != 0 || len(items) != 0 {
		t.Errorf("want empty list, got total=%d items=%d", total, len(items))
	}
}

func TestListClusters_RequiresAuth(t *testing.T) {
	srv, _ := newClusterServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/clusters", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

func TestListClusters_ReturnsSeedData(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create two clusters first.
	for _, name := range []string{"cluster-a", "cluster-b"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/clusters",
			jsonBody(t, map[string]any{"name": name, "lxd_endpoint": "https://lxd.example.com:8443"}))
		req.Header.Set("Authorization", authHeader(key))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed create %q: want 201, got %d", name, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/clusters", nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	_, total := decodeClusterList(t, rec)
	if total != 2 {
		t.Errorf("total: want 2, got %d", total)
	}
}

// ─── Create Cluster ───────────────────────────────────────────────────────────

func TestCreateCluster_Success(t *testing.T) {
	srv, key := newClusterServer(t)

	body := map[string]any{
		"name":                 "prod-eu-central",
		"lxd_endpoint":         "https://lxd.prod-eu-central.example.internal:8443",
		"hyperscaler_provider": "hetzner",
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %s", ct)
	}

	c := decodeCluster(t, rec)
	if c.ID == "" {
		t.Error("id must be set by the server")
	}
	if c.Name != "prod-eu-central" {
		t.Errorf("name: want prod-eu-central, got %s", c.Name)
	}
	if c.LXDEndpoint != "https://lxd.prod-eu-central.example.internal:8443" {
		t.Errorf("lxd_endpoint: got %s", c.LXDEndpoint)
	}
	if c.Status != "active" {
		t.Errorf("status: want active, got %s", c.Status)
	}
	if c.CreatedAt.IsZero() {
		t.Error("created_at must be set")
	}
}

func TestCreateCluster_DefaultsApplied(t *testing.T) {
	srv, key := newClusterServer(t)

	body := map[string]any{
		"name":         "minimal-cluster",
		"lxd_endpoint": "https://lxd.example.com:8443",
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", rec.Code)
	}
	c := decodeCluster(t, rec)
	if c.HyperscalerProvider != "hetzner" {
		t.Errorf("hyperscaler_provider default: want hetzner, got %s", c.HyperscalerProvider)
	}
	if c.HyperscalerConfig == nil {
		t.Error("hyperscaler_config should default to empty map")
	}
	if c.ScalingConfig == nil {
		t.Error("scaling_config should default to empty map")
	}
}

func TestCreateCluster_MissingName_Returns422(t *testing.T) {
	srv, key := newClusterServer(t)

	body := map[string]any{"lxd_endpoint": "https://lxd.example.com:8443"}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: want 422, got %d", rec.Code)
	}
}

func TestCreateCluster_MissingEndpoint_Returns422(t *testing.T) {
	srv, key := newClusterServer(t)

	body := map[string]any{"name": "my-cluster"}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: want 422, got %d", rec.Code)
	}
}

func TestCreateCluster_InvalidJSON_Returns422(t *testing.T) {
	srv, key := newClusterServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		bytes.NewReader([]byte(`not-json`)))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: want 422, got %d", rec.Code)
	}
}

func TestCreateCluster_DuplicateName_Returns409(t *testing.T) {
	srv, key := newClusterServer(t)

	body := map[string]any{"name": "dup-cluster", "lxd_endpoint": "https://lxd.example.com:8443"}
	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
		req.Header.Set("Authorization", authHeader(key))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		want := http.StatusCreated
		if i == 1 {
			want = http.StatusConflict
		}
		if rec.Code != want {
			t.Errorf("attempt %d: want %d, got %d", i, want, rec.Code)
		}
	}
}

func TestCreateCluster_RequiresAuth(t *testing.T) {
	srv, _ := newClusterServer(t)

	body := map[string]any{"name": "x", "lxd_endpoint": "https://lxd.example.com:8443"}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters", jsonBody(t, body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

// ─── Get Cluster ──────────────────────────────────────────────────────────────

func TestGetCluster_Success(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create a cluster.
	createReq := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		jsonBody(t, map[string]any{"name": "get-test", "lxd_endpoint": "https://lxd.example.com:8443"}))
	createReq.Header.Set("Authorization", authHeader(key))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: create cluster: want 201, got %d", createRec.Code)
	}
	created := decodeCluster(t, createRec)

	// Fetch by ID.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/clusters/%s", created.ID), nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	got := decodeCluster(t, rec)
	if got.ID != created.ID {
		t.Errorf("id: want %s, got %s", created.ID, got.ID)
	}
}

func TestGetCluster_NotFound_Returns404(t *testing.T) {
	srv, key := newClusterServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/clusters/nonexistent-id", nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", rec.Code)
	}
}

func TestGetCluster_RequiresAuth(t *testing.T) {
	srv, _ := newClusterServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/clusters/some-id", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

// ─── Update Cluster ───────────────────────────────────────────────────────────

func TestUpdateCluster_Success(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create.
	createReq := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		jsonBody(t, map[string]any{"name": "update-test", "lxd_endpoint": "https://old.example.com:8443"}))
	createReq.Header.Set("Authorization", authHeader(key))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: want 201, got %d", createRec.Code)
	}
	created := decodeCluster(t, createRec)

	// Update name and endpoint.
	updateBody := map[string]any{
		"name":         "update-test-renamed",
		"lxd_endpoint": "https://new.example.com:8443",
	}
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/v1/clusters/%s", created.ID), jsonBody(t, updateBody))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	updated := decodeCluster(t, rec)
	if updated.Name != "update-test-renamed" {
		t.Errorf("name: want update-test-renamed, got %s", updated.Name)
	}
	if updated.LXDEndpoint != "https://new.example.com:8443" {
		t.Errorf("lxd_endpoint: want https://new.example.com:8443, got %s", updated.LXDEndpoint)
	}
}

func TestUpdateCluster_PartialUpdate(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create.
	createReq := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		jsonBody(t, map[string]any{
			"name":                 "partial-test",
			"lxd_endpoint":         "https://lxd.example.com:8443",
			"hyperscaler_provider": "hetzner",
		}))
	createReq.Header.Set("Authorization", authHeader(key))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: want 201, got %d", createRec.Code)
	}
	created := decodeCluster(t, createRec)

	// Update only status, leaving other fields unchanged.
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/v1/clusters/%s", created.ID),
		jsonBody(t, map[string]any{"status": "inactive"}))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	updated := decodeCluster(t, rec)
	if updated.Status != "inactive" {
		t.Errorf("status: want inactive, got %s", updated.Status)
	}
	// Other fields must be preserved.
	if updated.Name != created.Name {
		t.Errorf("name must not change: want %s, got %s", created.Name, updated.Name)
	}
	if updated.HyperscalerProvider != created.HyperscalerProvider {
		t.Errorf("hyperscaler_provider must not change")
	}
}

func TestUpdateCluster_NotFound_Returns404(t *testing.T) {
	srv, key := newClusterServer(t)

	req := httptest.NewRequest(http.MethodPut, "/v1/clusters/no-such-id",
		jsonBody(t, map[string]any{"name": "new-name"}))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", rec.Code)
	}
}

func TestUpdateCluster_ConflictName_Returns409(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create two clusters.
	for _, name := range []string{"alpha", "beta"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/clusters",
			jsonBody(t, map[string]any{"name": name, "lxd_endpoint": "https://lxd.example.com:8443"}))
		req.Header.Set("Authorization", authHeader(key))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed %q: want 201, got %d", name, rec.Code)
		}
	}

	// Fetch the ID of "alpha".
	listReq := httptest.NewRequest(http.MethodGet, "/v1/clusters", nil)
	listReq.Header.Set("Authorization", authHeader(key))
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)
	items, _ := decodeClusterList(t, listRec)

	var alphaID string
	for _, c := range items {
		if c.Name == "alpha" {
			alphaID = c.ID
		}
	}
	if alphaID == "" {
		t.Fatal("could not find cluster alpha")
	}

	// Try to rename alpha → beta (conflict).
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/v1/clusters/%s", alphaID),
		jsonBody(t, map[string]any{"name": "beta"}))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: want 409, got %d", rec.Code)
	}
}

func TestUpdateCluster_InvalidJSON_Returns422(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create a cluster to have a valid ID.
	createReq := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		jsonBody(t, map[string]any{"name": "json-test", "lxd_endpoint": "https://lxd.example.com:8443"}))
	createReq.Header.Set("Authorization", authHeader(key))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	created := decodeCluster(t, createRec)

	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/v1/clusters/%s", created.ID),
		bytes.NewReader([]byte(`not-json`)))
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: want 422, got %d", rec.Code)
	}
}

func TestUpdateCluster_RequiresAuth(t *testing.T) {
	srv, _ := newClusterServer(t)

	req := httptest.NewRequest(http.MethodPut, "/v1/clusters/some-id",
		jsonBody(t, map[string]any{"name": "x"}))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

// ─── Delete Cluster ───────────────────────────────────────────────────────────

func TestDeleteCluster_Success(t *testing.T) {
	srv, key := newClusterServer(t)

	// Create.
	createReq := httptest.NewRequest(http.MethodPost, "/v1/clusters",
		jsonBody(t, map[string]any{"name": "delete-test", "lxd_endpoint": "https://lxd.example.com:8443"}))
	createReq.Header.Set("Authorization", authHeader(key))
	createRec := httptest.NewRecorder()
	srv.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: want 201, got %d", createRec.Code)
	}
	created := decodeCluster(t, createRec)

	// Delete.
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/v1/clusters/%s", created.ID), nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", rec.Code)
	}

	// Confirm it's gone.
	getReq := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/v1/clusters/%s", created.ID), nil)
	getReq.Header.Set("Authorization", authHeader(key))
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("after delete: want 404, got %d", getRec.Code)
	}
}

func TestDeleteCluster_NotFound_Returns404(t *testing.T) {
	srv, key := newClusterServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/clusters/no-such-id", nil)
	req.Header.Set("Authorization", authHeader(key))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", rec.Code)
	}
}

func TestDeleteCluster_RequiresAuth(t *testing.T) {
	srv, _ := newClusterServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/v1/clusters/some-id", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", rec.Code)
	}
}

// ─── Health still works with cluster routes registered ────────────────────────

func TestHealthEndpoint_WithClusterRepo(t *testing.T) {
	srv, _ := newClusterServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("health: want 200, got %d", rec.Code)
	}
}
