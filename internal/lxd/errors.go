package lxd

import "errors"

// ErrNodeNotFound is returned by GetClusterMember and GetNodeResources when the
// requested cluster member does not exist. Callers should treat this as a
// definitive "gone" signal and update their local state accordingly rather than
// retrying the lookup.
var ErrNodeNotFound = errors.New("lxd: node not found")

// ErrInstanceNotFound is returned by GetInstance and MoveInstance when the
// requested instance does not exist. This error is not retriable; the caller
// must verify the instance name before attempting the operation again.
var ErrInstanceNotFound = errors.New("lxd: instance not found")

// ErrUnreachable is returned when the LXD endpoint cannot be contacted, for
// example because the network is unavailable or the address is wrong. Callers
// may retry the operation after a back-off period.
var ErrUnreachable = errors.New("lxd: endpoint unreachable")

// ErrMigrationFailed is returned by MoveInstance when the LXD live-migration
// operation completes but reports a failure. The wrapped error carries the
// reason reported by LXD. Callers should log the error, mark the migration as
// failed, and abort scale-in for that node (ADR-007).
var ErrMigrationFailed = errors.New("lxd: migration failed")

// ErrClusterAlreadyBootstrapped is returned by InitCluster and JoinCluster
// when the target node is already a member of an LXD cluster. Callers should
// treat this as a successful no-op; re-running bootstrap against an
// already-initialised cluster must not return an error to the operator.
var ErrClusterAlreadyBootstrapped = errors.New("lxd: cluster already bootstrapped")
