package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// clusterCreateRequest is the decoded body for POST /v1/clusters.
type clusterCreateRequest struct {
	Name                string         `json:"name"`
	LXDEndpoint         string         `json:"lxd_endpoint"`
	HyperscalerProvider string         `json:"hyperscaler_provider"`
	HyperscalerConfig   map[string]any `json:"hyperscaler_config"`
	ScalingConfig       map[string]any `json:"scaling_config"`
}

// clusterUpdateRequest is the decoded body for PUT /v1/clusters/{cluster_id}.
// All fields are optional; absent fields retain their current values.
type clusterUpdateRequest struct {
	Name                *string        `json:"name"`
	LXDEndpoint         *string        `json:"lxd_endpoint"`
	HyperscalerProvider *string        `json:"hyperscaler_provider"`
	HyperscalerConfig   map[string]any `json:"hyperscaler_config"`
	ScalingConfig       map[string]any `json:"scaling_config"`
	Status              *string        `json:"status"`
}

// clusterListResponse is the envelope for GET /v1/clusters.
type clusterListResponse struct {
	Items []*model.Cluster `json:"items"`
	Total int              `json:"total"`
}

// handleListClusters handles GET /v1/clusters.
func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := s.clusters.ListClusters(r.Context())
	if err != nil {
		s.logger.Error("list clusters", slog.String("error", err.Error()),
			slog.String("request_id", RequestIDFromContext(r.Context())))
		writeInternalError(w)
		return
	}

	if clusters == nil {
		clusters = []*model.Cluster{}
	}

	resp := clusterListResponse{Items: clusters, Total: len(clusters)}
	writeJSON(w, http.StatusOK, resp)
}

// handleCreateCluster handles POST /v1/clusters.
func (s *Server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	var req clusterCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "request body is not valid JSON")
		return
	}

	// Validate required fields.
	req.Name = strings.TrimSpace(req.Name)
	req.LXDEndpoint = strings.TrimSpace(req.LXDEndpoint)

	if req.Name == "" || req.LXDEndpoint == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "name and lxd_endpoint are required")
		return
	}

	// Apply defaults.
	if req.HyperscalerProvider == "" {
		req.HyperscalerProvider = "hetzner"
	}
	if req.HyperscalerConfig == nil {
		req.HyperscalerConfig = map[string]any{}
	}
	if req.ScalingConfig == nil {
		req.ScalingConfig = map[string]any{}
	}

	c := &model.Cluster{
		Name:                req.Name,
		LXDEndpoint:         req.LXDEndpoint,
		HyperscalerProvider: req.HyperscalerProvider,
		HyperscalerConfig:   req.HyperscalerConfig,
		ScalingConfig:       req.ScalingConfig,
		Status:              "active",
	}

	created, err := s.clusters.CreateCluster(r.Context(), c)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrConflict):
			writeError(w, http.StatusConflict, "conflict", "a cluster with that name already exists")
		default:
			s.logger.Error("create cluster", slog.String("error", err.Error()),
				slog.String("request_id", RequestIDFromContext(r.Context())))
			writeInternalError(w)
		}
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleGetCluster handles GET /v1/clusters/{cluster_id}.
func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("cluster_id")

	cluster, err := s.clusters.GetCluster(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
		default:
			s.logger.Error("get cluster", slog.String("error", err.Error()),
				slog.String("request_id", RequestIDFromContext(r.Context())))
			writeInternalError(w)
		}
		return
	}

	writeJSON(w, http.StatusOK, cluster)
}

// handleUpdateCluster handles PUT /v1/clusters/{cluster_id}.
func (s *Server) handleUpdateCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("cluster_id")

	var req clusterUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "request body is not valid JSON")
		return
	}

	// Fetch the current cluster to apply partial updates.
	existing, err := s.clusters.GetCluster(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
		default:
			s.logger.Error("update cluster: get existing", slog.String("error", err.Error()),
				slog.String("request_id", RequestIDFromContext(r.Context())))
			writeInternalError(w)
		}
		return
	}

	// Merge provided fields onto the existing cluster.
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "name must not be empty")
			return
		}
		existing.Name = name
	}
	if req.LXDEndpoint != nil {
		endpoint := strings.TrimSpace(*req.LXDEndpoint)
		if endpoint == "" {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "lxd_endpoint must not be empty")
			return
		}
		existing.LXDEndpoint = endpoint
	}
	if req.HyperscalerProvider != nil {
		existing.HyperscalerProvider = *req.HyperscalerProvider
	}
	if req.HyperscalerConfig != nil {
		existing.HyperscalerConfig = req.HyperscalerConfig
	}
	if req.ScalingConfig != nil {
		existing.ScalingConfig = req.ScalingConfig
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}

	updated, err := s.clusters.UpdateCluster(r.Context(), existing)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
		case errors.Is(err, persistence.ErrConflict):
			writeError(w, http.StatusConflict, "conflict", "a cluster with that name already exists")
		default:
			s.logger.Error("update cluster", slog.String("error", err.Error()),
				slog.String("request_id", RequestIDFromContext(r.Context())))
			writeInternalError(w)
		}
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteCluster handles DELETE /v1/clusters/{cluster_id}.
func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("cluster_id")

	if err := s.clusters.DeleteCluster(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, persistence.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "cluster not found")
		default:
			s.logger.Error("delete cluster", slog.String("error", err.Error()),
				slog.String("request_id", RequestIDFromContext(r.Context())))
			writeInternalError(w)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
