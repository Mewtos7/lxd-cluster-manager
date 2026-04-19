// Package lxd provides the LXD client integration layer described in
// ADR-006 and ADR-007. The [Client] interface abstracts all LXD REST API
// calls needed by the inventory-sync, live-migration, and orchestration
// stories. The concrete implementation ([lxdClient]) communicates with LXD
// over HTTPS using the standard net/http package — no external LXD SDK is
// required. An in-memory fake implementation lives in the [fake] sub-package
// for use in unit tests.
//
// # Connection configuration
//
// Use [New] to create a client pointing at an LXD cluster endpoint. Functional
// options control TLS behaviour and the underlying HTTP transport:
//
//   - [WithInsecureSkipVerify] — skip TLS certificate verification. Intended
//     for development and testing against LXD nodes with self-signed certs.
//   - [WithClientCertificate] — attach a PEM-encoded client certificate and
//     private key for mutual TLS authentication.
//   - [WithServerCA] — trust the supplied PEM-encoded CA certificate when
//     verifying the LXD server's certificate.
//   - [WithHTTPClient] — inject a custom *http.Client (useful in tests).
//
// # LXD REST API
//
// The implementation talks to the LXD REST API v1.0. Synchronous responses
// are returned directly. Asynchronous responses (type "async") are awaited by
// polling /1.0/operations/{id}/wait until the operation reaches a terminal
// state or ctx is cancelled.
package lxd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── LXD REST API wire types ─────────────────────────────────────────────────

// apiResponse is the top-level envelope for all LXD REST API responses.
type apiResponse struct {
	// Type is "sync" for immediately available results or "async" for
	// operations that complete in the background.
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Operation  string          `json:"operation"`
	Error      string          `json:"error"`
	ErrorCode  int             `json:"error_code"`
	Metadata   json.RawMessage `json:"metadata"`
}

// apiMember maps to a single LXD cluster member object from
// GET /1.0/cluster/members?recursion=1 or GET /1.0/cluster/members/{name}.
type apiMember struct {
	ServerName   string   `json:"server_name"`
	URL          string   `json:"url"`
	Status       string   `json:"status"`
	Message      string   `json:"message"`
	Architecture string   `json:"architecture"`
	Description  string   `json:"description"`
	Roles        []string `json:"roles"`
}

// apiInstance maps to a single LXD instance object from
// GET /1.0/instances?recursion=1 or GET /1.0/instances/{name}.
type apiInstance struct {
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Type        string            `json:"type"`
	Location    string            `json:"location"`
	Description string            `json:"description"`
	Config      map[string]string `json:"config"`
}

// apiResources maps to the LXD resources response from
// GET /1.0/resources?target={node}.
type apiResources struct {
	CPU struct {
		Total uint64 `json:"total"`
	} `json:"cpu"`
	Memory struct {
		Total uint64 `json:"total"`
		Used  uint64 `json:"used"`
	} `json:"memory"`
	Storage struct {
		Disks []apiDisk `json:"disks"`
	} `json:"storage"`
}

// apiDisk represents a single disk in the LXD resources response.
type apiDisk struct {
	Size uint64 `json:"size"`
	// Partitions holds per-partition usage data when available.
	Partitions []apiPartition `json:"partitions"`
}

// apiPartition represents a single partition in the LXD resources response.
type apiPartition struct {
	Size uint64 `json:"size"`
	Used uint64 `json:"used"`
}

// apiOperation maps to the LXD async operation metadata.
type apiOperation struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	Err        string `json:"err"`
}

// apiMoveRequest is the JSON body sent to POST /1.0/instances/{name} to
// initiate a within-cluster live migration (ADR-007).
type apiMoveRequest struct {
	Migration bool   `json:"migration"`
	Live      bool   `json:"live"`
	Target    string `json:"target,omitempty"`
}

// ─── Client construction ──────────────────────────────────────────────────────

// lxdClient is the concrete implementation of [Client]. It communicates with
// the LXD REST API over HTTPS.
type lxdClient struct {
	endpoint   string       // base URL, e.g. "https://192.168.1.1:8443"
	httpClient *http.Client // HTTP client with configured TLS transport
}

// Compile-time assertion: lxdClient must satisfy Client.
var _ Client = (*lxdClient)(nil)

// Option is a functional option for configuring a [lxdClient] at construction
// time.
type Option func(*lxdClient) error

// WithInsecureSkipVerify configures the client to skip TLS certificate
// verification when connecting to the LXD endpoint. This is intended for
// development and testing against LXD nodes that use self-signed certificates.
// Do not use in production.
func WithInsecureSkipVerify() Option {
	return func(c *lxdClient) error {
		transport := cloneTransport(c.httpClient)
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{} //nolint:gosec // intentionally configurable
		}
		transport.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // opt-in for dev/test
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   c.httpClient.Timeout,
		}
		return nil
	}
}

// WithClientCertificate configures mutual TLS authentication using the supplied
// PEM-encoded certificate and private key. LXD uses client certificates as the
// primary authentication mechanism.
func WithClientCertificate(certPEM, keyPEM []byte) Option {
	return func(c *lxdClient) error {
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("lxd: parse client certificate: %w", err)
		}
		transport := cloneTransport(c.httpClient)
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.Certificates = append(
			transport.TLSClientConfig.Certificates, cert,
		)
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   c.httpClient.Timeout,
		}
		return nil
	}
}

// WithServerCA configures the client to trust the supplied PEM-encoded CA
// certificate when verifying the LXD server's TLS certificate. Use this when
// LXD nodes use certificates signed by a private CA.
func WithServerCA(caPEM []byte) Option {
	return func(c *lxdClient) error {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return fmt.Errorf("lxd: parse CA certificate: no valid certificates found in PEM")
		}
		transport := cloneTransport(c.httpClient)
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.RootCAs = pool
		c.httpClient = &http.Client{
			Transport: transport,
			Timeout:   c.httpClient.Timeout,
		}
		return nil
	}
}

// WithHTTPClient replaces the underlying HTTP client. Intended for testing
// (e.g. wrapping httptest.Server).
func WithHTTPClient(client *http.Client) Option {
	return func(c *lxdClient) error {
		c.httpClient = client
		return nil
	}
}

// New creates a Client that communicates with the LXD cluster at endpoint.
// endpoint must be a non-empty URL with scheme and host, e.g.
// "https://192.168.1.1:8443". Trailing slashes are stripped automatically.
//
// Options are applied in order; conflicting options are resolved by the last
// one applied winning.
func New(endpoint string, opts ...Option) (Client, error) {
	if endpoint == "" {
		return nil, errors.New("lxd: endpoint must not be empty")
	}
	c := &lxdClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// ─── Client operations ────────────────────────────────────────────────────────

// GetClusterMembers returns information about all members in the LXD cluster.
func (c *lxdClient) GetClusterMembers(ctx context.Context) ([]NodeInfo, error) {
	var members []apiMember
	if err := c.getJSON(ctx, "/1.0/cluster/members?recursion=1", &members); err != nil {
		return nil, fmt.Errorf("lxd: get cluster members: %w", err)
	}
	out := make([]NodeInfo, 0, len(members))
	for _, m := range members {
		out = append(out, memberToNodeInfo(m))
	}
	return out, nil
}

// GetClusterMember returns the current state of the named cluster member.
func (c *lxdClient) GetClusterMember(ctx context.Context, name string) (*NodeInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("lxd: get cluster member: name must not be empty")
	}
	var member apiMember
	if err := c.getJSON(ctx, "/1.0/cluster/members/"+name, &member); err != nil {
		return nil, fmt.Errorf("lxd: get cluster member %q: %w", name, err)
	}
	info := memberToNodeInfo(member)
	return &info, nil
}

// GetNodeResources returns resource capacity information for the named cluster
// member.
func (c *lxdClient) GetNodeResources(ctx context.Context, nodeName string) (*NodeResources, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("lxd: get node resources: nodeName must not be empty")
	}
	var res apiResources
	if err := c.getJSON(ctx, "/1.0/resources?target="+url.QueryEscape(nodeName), &res); err != nil {
		return nil, fmt.Errorf("lxd: get node resources for %q: %w", nodeName, err)
	}
	return resourcesToNodeResources(res), nil
}

// ListInstances returns all instances managed by the LXD cluster.
func (c *lxdClient) ListInstances(ctx context.Context) ([]InstanceInfo, error) {
	var instances []apiInstance
	if err := c.getJSON(ctx, "/1.0/instances?recursion=1", &instances); err != nil {
		return nil, fmt.Errorf("lxd: list instances: %w", err)
	}
	out := make([]InstanceInfo, 0, len(instances))
	for _, i := range instances {
		out = append(out, instanceToInfo(i))
	}
	return out, nil
}

// GetInstance returns the current state of the named instance.
func (c *lxdClient) GetInstance(ctx context.Context, name string) (*InstanceInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("lxd: get instance: name must not be empty")
	}
	var inst apiInstance
	if err := c.getJSON(ctx, "/1.0/instances/"+name, &inst); err != nil {
		return nil, fmt.Errorf("lxd: get instance %q: %w", name, err)
	}
	info := instanceToInfo(inst)
	return &info, nil
}

// MoveInstance live-migrates the named instance to the specified target cluster
// member. The method blocks until the operation completes or ctx is cancelled.
func (c *lxdClient) MoveInstance(ctx context.Context, instanceName, targetNode string) error {
	if instanceName == "" {
		return fmt.Errorf("lxd: move instance: instanceName must not be empty")
	}
	if targetNode == "" {
		return fmt.Errorf("lxd: move instance: targetNode must not be empty")
	}

	body := apiMoveRequest{
		Migration: true,
		Live:      true,
		Target:    targetNode,
	}
	operationPath, err := c.postJSON(ctx, "/1.0/instances/"+url.PathEscape(instanceName)+"?target="+url.QueryEscape(targetNode), body)
	if err != nil {
		return fmt.Errorf("lxd: move instance %q to %q: %w", instanceName, targetNode, err)
	}

	if err := c.waitOperation(ctx, operationPath); err != nil {
		return fmt.Errorf("lxd: move instance %q to %q: %w", instanceName, targetNode, err)
	}
	return nil
}

// ─── Internal HTTP helpers ────────────────────────────────────────────────────

// getJSON performs a GET request to the given path (relative to the endpoint)
// and decodes the LXD synchronous response's metadata field into out.
// It wraps known LXD error status codes into sentinel errors.
func (c *lxdClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %s", ErrUnreachable, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.decodeSync(resp, out)
}

// postJSON performs a POST request to the given path with a JSON-encoded body.
// For synchronous responses it decodes metadata into out (if non-nil).
// For asynchronous responses it returns the operation path so the caller can
// wait on it.
func (c *lxdClient) postJSON(ctx context.Context, path string, body any) (operationPath string, err error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("lxd: marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("%w: build request: %s", ErrUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("lxd: read response body: %w", err)
	}

	var envelope apiResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", fmt.Errorf("lxd: decode response envelope: %w", err)
	}

	if err := c.checkAPIError(envelope, resp.StatusCode); err != nil {
		return "", err
	}

	if envelope.Type == "async" {
		return envelope.Operation, nil
	}
	return "", nil
}

// waitOperation polls /1.0/operations/{id}/wait until the operation reaches a
// terminal state (Success or Failure) or ctx is cancelled.
func (c *lxdClient) waitOperation(ctx context.Context, operationPath string) error {
	if operationPath == "" {
		return nil
	}
	// Strip leading slash if present so we don't double-slash.
	waitPath := strings.TrimPrefix(operationPath, "/") + "/wait"

	var op apiOperation
	if err := c.getJSON(ctx, "/"+waitPath, &op); err != nil {
		return fmt.Errorf("wait for operation: %w", err)
	}
	if op.StatusCode >= 400 || op.Err != "" {
		msg := op.Err
		if msg == "" {
			msg = op.Status
		}
		return fmt.Errorf("%w: %s", ErrMigrationFailed, msg)
	}
	return nil
}

// decodeSync reads resp.Body, unmarshals the LXD response envelope, checks for
// API-level errors, and decodes the metadata field into out.
func (c *lxdClient) decodeSync(resp *http.Response, out any) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("lxd: read response body: %w", err)
	}

	var envelope apiResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("lxd: decode response envelope: %w", err)
	}

	if err := c.checkAPIError(envelope, resp.StatusCode); err != nil {
		return err
	}

	if out != nil {
		if err := json.Unmarshal(envelope.Metadata, out); err != nil {
			return fmt.Errorf("lxd: decode response metadata: %w", err)
		}
	}
	return nil
}

// checkAPIError maps LXD API error codes to sentinel errors. It is called
// after decoding the response envelope.
func (c *lxdClient) checkAPIError(env apiResponse, httpStatus int) error {
	if env.ErrorCode != 0 || (httpStatus >= 400 && env.Error != "") {
		switch {
		case env.ErrorCode == 404 || httpStatus == 404:
			return fmt.Errorf("%w: %s", ErrNodeNotFound, env.Error)
		case httpStatus == 0:
			return fmt.Errorf("%w: %s", ErrUnreachable, env.Error)
		default:
			return fmt.Errorf("lxd: API error %d: %s", env.ErrorCode, env.Error)
		}
	}
	if httpStatus >= 400 {
		return fmt.Errorf("lxd: HTTP %d", httpStatus)
	}
	return nil
}

// ─── Mapping helpers ──────────────────────────────────────────────────────────

func memberToNodeInfo(m apiMember) NodeInfo {
	return NodeInfo{
		Name:         m.ServerName,
		URL:          m.URL,
		Status:       m.Status,
		Message:      m.Message,
		Architecture: m.Architecture,
		Description:  m.Description,
		Roles:        m.Roles,
	}
}

func instanceToInfo(i apiInstance) InstanceInfo {
	return InstanceInfo{
		Name:         i.Name,
		Status:       i.Status,
		InstanceType: i.Type,
		Location:     i.Location,
		Description:  i.Description,
		Config:       i.Config,
	}
}

func resourcesToNodeResources(r apiResources) *NodeResources {
	var diskTotal, diskUsed uint64
	for _, d := range r.Storage.Disks {
		diskTotal += d.Size
		for _, p := range d.Partitions {
			diskUsed += p.Used
		}
	}
	return &NodeResources{
		CPU:    CPUResources{Total: r.CPU.Total},
		Memory: MemoryResources{Total: r.Memory.Total, Used: r.Memory.Used},
		Disk:   DiskResources{Total: diskTotal, Used: diskUsed},
	}
}

// cloneTransport returns a shallow copy of the HTTP transport from client, or
// a new *http.Transport if client has no transport set. This avoids mutating
// shared transports when applying options.
func cloneTransport(client *http.Client) *http.Transport {
	if client.Transport == nil {
		return &http.Transport{}
	}
	if t, ok := client.Transport.(*http.Transport); ok {
		clone := t.Clone()
		return clone
	}
	// Fallback: create a fresh transport; the custom transport is preserved
	// only via WithHTTPClient.
	return &http.Transport{}
}
