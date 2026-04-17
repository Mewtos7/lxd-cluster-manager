-- Migration: 0001_initial_schema
-- Creates the relational schema for clusters, nodes, instances, placements,
-- scaling actions, migrations, and audit events.

-- ---------------------------------------------------------------------------
-- Helper: auto-update updated_at on every row change
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- clusters
-- Represents a single LXD cluster managed by this service.
-- ---------------------------------------------------------------------------

CREATE TABLE clusters (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 TEXT        NOT NULL UNIQUE,
    lxd_endpoint         TEXT        NOT NULL,
    -- TLS material stored encrypted at rest; the application layer handles
    -- encryption/decryption before persisting and after reading.
    lxd_tls_cert         BYTEA,
    lxd_tls_key          BYTEA,
    lxd_server_cert      BYTEA,
    hyperscaler_provider TEXT        NOT NULL DEFAULT 'hetzner',
    -- Free-form provider config (API token reference, region, server type, …)
    hyperscaler_config   JSONB       NOT NULL DEFAULT '{}',
    -- Scaling thresholds: high_watermark, low_watermark, sustained_seconds,
    -- cooldown_seconds, loop_interval_seconds, etc.
    scaling_config       JSONB       NOT NULL DEFAULT '{}',
    status               TEXT        NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive', 'error')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER set_clusters_updated_at
    BEFORE UPDATE ON clusters
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ---------------------------------------------------------------------------
-- nodes
-- A physical or virtual server that is a member of a cluster.
-- ---------------------------------------------------------------------------

CREATE TABLE nodes (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id            UUID        NOT NULL REFERENCES clusters (id) ON DELETE CASCADE,
    name                  TEXT        NOT NULL,
    lxd_member_name       TEXT        NOT NULL,
    -- Identifier on the hyperscaler (e.g. Hetzner server ID) for deprovisioning.
    hyperscaler_server_id TEXT,
    cpu_cores             INTEGER     NOT NULL CHECK (cpu_cores > 0),
    memory_bytes          BIGINT      NOT NULL CHECK (memory_bytes > 0),
    disk_bytes            BIGINT      NOT NULL CHECK (disk_bytes > 0),
    status                TEXT        NOT NULL DEFAULT 'provisioning'
        CHECK (status IN (
            'provisioning',
            'online',
            'offline',
            'draining',
            'deprovisioning',
            'error'
        )),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, name)
);

CREATE INDEX idx_nodes_cluster_id ON nodes (cluster_id);
CREATE INDEX idx_nodes_status     ON nodes (status);

CREATE TRIGGER set_nodes_updated_at
    BEFORE UPDATE ON nodes
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ---------------------------------------------------------------------------
-- instances
-- A container or VM managed within a cluster.
-- ---------------------------------------------------------------------------

CREATE TABLE instances (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID        NOT NULL REFERENCES clusters (id) ON DELETE CASCADE,
    -- Current node; NULL while the instance is pending placement or deleted.
    node_id         UUID        REFERENCES nodes (id) ON DELETE SET NULL,
    name            TEXT        NOT NULL,
    instance_type   TEXT        NOT NULL DEFAULT 'container'
        CHECK (instance_type IN ('container', 'vm')),
    status          TEXT        NOT NULL DEFAULT 'pending'
        CHECK (status IN (
            'pending',
            'starting',
            'running',
            'stopping',
            'stopped',
            'migrating',
            'error',
            'deleted'
        )),
    -- Resource requests used for scheduling decisions.
    cpu_limit       INTEGER     NOT NULL DEFAULT 1 CHECK (cpu_limit > 0),
    memory_limit    BIGINT      NOT NULL DEFAULT 512 * 1024 * 1024   CHECK (memory_limit > 0),  -- 512 MiB
    disk_limit      BIGINT      NOT NULL DEFAULT 10 * 1024 * 1024 * 1024 CHECK (disk_limit > 0), -- 10 GiB
    -- Arbitrary LXD instance configuration key/value pairs.
    config          JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, name)
);

CREATE INDEX idx_instances_cluster_id ON instances (cluster_id);
CREATE INDEX idx_instances_node_id    ON instances (node_id);
CREATE INDEX idx_instances_status     ON instances (status);

CREATE TRIGGER set_instances_updated_at
    BEFORE UPDATE ON instances
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ---------------------------------------------------------------------------
-- placements
-- Immutable history of where each instance has been placed (and evicted from).
-- ---------------------------------------------------------------------------

CREATE TABLE placements (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id      UUID        NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    node_id          UUID        NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    placed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    evicted_at       TIMESTAMPTZ,
    placement_reason TEXT,
    eviction_reason  TEXT
);

CREATE INDEX idx_placements_instance_id ON placements (instance_id);
CREATE INDEX idx_placements_node_id     ON placements (node_id);

-- ---------------------------------------------------------------------------
-- scaling_actions
-- Records every scale-out or scale-in decision made by the reconciliation loop.
-- ---------------------------------------------------------------------------

CREATE TABLE scaling_actions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id     UUID        NOT NULL REFERENCES clusters (id) ON DELETE CASCADE,
    action_type    TEXT        NOT NULL CHECK (action_type IN ('scale_out', 'scale_in')),
    trigger_reason TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'aborted')),
    -- For scale_in: the node being drained; for scale_out: the newly provisioned node.
    node_id        UUID        REFERENCES nodes (id) ON DELETE SET NULL,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    error_message  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scaling_actions_cluster_id ON scaling_actions (cluster_id);
CREATE INDEX idx_scaling_actions_status     ON scaling_actions (status);
CREATE INDEX idx_scaling_actions_node_id    ON scaling_actions (node_id);

CREATE TRIGGER set_scaling_actions_updated_at
    BEFORE UPDATE ON scaling_actions
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ---------------------------------------------------------------------------
-- migrations
-- Tracks individual live-migration operations (one per instance per move).
-- ---------------------------------------------------------------------------

CREATE TABLE migrations (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id       UUID        NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    scaling_action_id UUID        REFERENCES scaling_actions (id) ON DELETE SET NULL,
    source_node_id    UUID        NOT NULL REFERENCES nodes (id) ON DELETE RESTRICT,
    target_node_id    UUID        NOT NULL REFERENCES nodes (id) ON DELETE RESTRICT,
    status            TEXT        NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_migrations_instance_id       ON migrations (instance_id);
CREATE INDEX idx_migrations_scaling_action_id ON migrations (scaling_action_id);
CREATE INDEX idx_migrations_status            ON migrations (status);

CREATE TRIGGER set_migrations_updated_at
    BEFORE UPDATE ON migrations
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ---------------------------------------------------------------------------
-- audit_events
-- Append-only log of every state-changing operation for compliance and debug.
-- ---------------------------------------------------------------------------

CREATE TABLE audit_events (
    id            BIGSERIAL   PRIMARY KEY,
    cluster_id    UUID        REFERENCES clusters (id) ON DELETE SET NULL,
    -- 'cluster' | 'node' | 'instance' | 'scaling_action' | 'migration'
    resource_type TEXT        NOT NULL,
    resource_id   UUID,
    -- 'create' | 'update' | 'delete' | 'scale_out' | 'scale_in' | 'migrate' | …
    action        TEXT        NOT NULL,
    -- API key label or 'system' for reconciliation-loop actions.
    actor         TEXT,
    details       JSONB       NOT NULL DEFAULT '{}',
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_events_cluster_id            ON audit_events (cluster_id);
CREATE INDEX idx_audit_events_resource_type_and_id  ON audit_events (resource_type, resource_id);
CREATE INDEX idx_audit_events_occurred_at           ON audit_events (occurred_at DESC);
