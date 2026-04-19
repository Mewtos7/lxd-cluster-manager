package provider

import "errors"

// ErrServerNotFound is returned by GetServer and DeprovisionServer when the
// requested server ID does not exist in the provider's inventory. Callers
// should treat this as a definitive "gone" signal and update their local
// state accordingly rather than retrying the lookup.
var ErrServerNotFound = errors.New("server not found")

// ErrInvalidSpec is returned by ProvisionServer when the supplied ServerSpec
// contains invalid or missing required fields (e.g. an empty Name or Region).
// This error is not retriable; the caller must correct the spec before
// attempting to provision again.
var ErrInvalidSpec = errors.New("invalid server spec")
