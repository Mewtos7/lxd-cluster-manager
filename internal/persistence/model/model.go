// Package model defines the domain model structs that are passed between the
// persistence layer and the rest of the application. These structs mirror the
// database schema defined in db/migrations/0001_initial_schema.up.sql.
package model

import "time"

// Cluster represents a single LXD cluster managed by this service.
type Cluster struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	LXDEndpoint         string         `json:"lxd_endpoint"`
	HyperscalerProvider string         `json:"hyperscaler_provider"`
	HyperscalerConfig   map[string]any `json:"hyperscaler_config"`
	ScalingConfig       map[string]any `json:"scaling_config"`
	Status              string         `json:"status"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Node represents a physical or virtual server that is a member of a cluster.
type Node struct {
	ID                  string    `json:"id"`
	ClusterID           string    `json:"cluster_id"`
	Name                string    `json:"name"`
	LXDMemberName       string    `json:"lxd_member_name"`
	HyperscalerServerID string    `json:"hyperscaler_server_id,omitempty"`
	CPUCores            int       `json:"cpu_cores"`
	MemoryBytes         int64     `json:"memory_bytes"`
	DiskBytes           int64     `json:"disk_bytes"`
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Instance represents a container or VM managed within a cluster.
type Instance struct {
	ID           string         `json:"id"`
	ClusterID    string         `json:"cluster_id"`
	NodeID       string         `json:"node_id,omitempty"`
	Name         string         `json:"name"`
	InstanceType string         `json:"instance_type"`
	Status       string         `json:"status"`
	CPULimit     int            `json:"cpu_limit"`
	MemoryLimit  int64          `json:"memory_limit"`
	DiskLimit    int64          `json:"disk_limit"`
	Config       map[string]any `json:"config"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
