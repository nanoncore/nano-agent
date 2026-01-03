package agent

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client communicates with the Nanoncore control plane API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string

	// Agent-specific fields for na_ API keys
	agentID           string
	keyRotationNeeded bool
}

// EnrollRequest is sent to the control plane during enrollment.
type EnrollRequest struct {
	NodeID string            `json:"node_id"`
	Token  string            `json:"token"`
	Labels map[string]string `json:"labels,omitempty"`
	Roles  []string          `json:"roles,omitempty"` // ["olt", "bng"] - equipment types to manage
}

// EnrollResponse is returned from the control plane after enrollment.
type EnrollResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"private_key,omitempty"`
	CACert      string `json:"ca_cert,omitempty"`

	// Agent API key for per-agent rate limiting (na_ prefix)
	AgentAPIKey       string `json:"agent_api_key,omitempty"`
	AgentAPIKeyPrefix string `json:"agent_api_key_prefix,omitempty"`
	AgentID           string `json:"agent_id,omitempty"`

	// Organization/Network info (returned from enrollment)
	OrganizationID   string `json:"organization_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	NetworkID        string `json:"network_id,omitempty"`
	NetworkName      string `json:"network_name,omitempty"`
	NetworkSlug      string `json:"network_slug,omitempty"`
}

// NodeStatus represents the current status of the agent.
type NodeStatus struct {
	NodeID    string            `json:"node_id"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels,omitempty"`
	Version   string            `json:"version"`
	Uptime    string            `json:"uptime,omitempty"`
	LastSync  string            `json:"last_sync,omitempty"`
	VPPStatus *VPPStatus        `json:"vpp_status,omitempty"`
}

// VPPStatus contains VPP dataplane status.
type VPPStatus struct {
	Running       bool     `json:"running"`
	Version       string   `json:"version,omitempty"`
	Interfaces    int      `json:"interfaces"`
	InterfaceList []string `json:"interface_list,omitempty"`
}

// HeartbeatRequest is sent periodically to the control plane.
type HeartbeatRequest struct {
	NodeID    string     `json:"node_id"`
	Timestamp string     `json:"timestamp"`
	VPPStatus *VPPStatus `json:"vpp_status,omitempty"`
}

// HeartbeatResponse is returned from heartbeat calls.
type HeartbeatResponse struct {
	Acknowledged bool   `json:"acknowledged"`
	ConfigUpdate bool   `json:"config_update,omitempty"`
	Message      string `json:"message,omitempty"`
}

// NewClient creates a new API client.
func NewClient(baseURL string, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithMTLS creates a client with mutual TLS authentication.
func NewClientWithMTLS(baseURL, certFile, keyFile, caFile string) (*Client, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}, nil
}

// Enroll registers this node with the control plane using an enrollment token.
// This calls the token-based enrollment endpoint which looks up the token
// in the database to get the associated network/organization.
func (c *Client) Enroll(req *EnrollRequest) (*EnrollResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use the token-based enrollment endpoint
	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/nodes/enroll-token", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("enrollment failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var enrollResp EnrollResponse
	if err := json.Unmarshal(respBody, &enrollResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &enrollResp, nil
}

// Heartbeat sends a heartbeat to the control plane.
func (c *Client) Heartbeat(req *HeartbeatRequest) (*HeartbeatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/nodes/heartbeat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Use agent API key (na_) for per-agent rate limiting
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for server signals (e.g., key rotation required)
	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("heartbeat failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var hbResp HeartbeatResponse
	if err := json.Unmarshal(respBody, &hbResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &hbResp, nil
}

// GetConfig retrieves configuration from the control plane.
func (c *Client) GetConfig(nodeID string) (map[string]interface{}, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/api/v1/nodes/"+nodeID+"/config", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use agent API key (na_) for per-agent rate limiting
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for server signals (e.g., key rotation required)
	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get config failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var config map[string]interface{}
	if err := json.Unmarshal(respBody, &config); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return config, nil
}

// CheckAPIHealth verifies the control plane is reachable.
func (c *Client) CheckAPIHealth() error {
	resp, err := c.httpClient.Get(c.baseURL + "/healthz")
	if err != nil {
		return fmt.Errorf("failed to reach API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API unhealthy (HTTP %d)", resp.StatusCode)
	}

	return nil
}

// NewClientWithAPIKey creates a client with API key authentication.
func NewClientWithAPIKey(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ValidateAPIKeyResponse is returned from API key validation.
type ValidateAPIKeyResponse struct {
	Valid     bool   `json:"valid"`
	UserID    string `json:"user_id,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ValidateAPIKey validates an API key with the control plane.
func (c *Client) ValidateAPIKey() (*ValidateAPIKeyResponse, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/api/v1/auth/validate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return &ValidateAPIKeyResponse{Valid: false, Message: "Invalid API key"}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validation failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var validateResp ValidateAPIKeyResponse
	if err := json.Unmarshal(respBody, &validateResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &validateResp, nil
}

// ListOrganizationsResponse is returned from listing organizations.
type ListOrganizationsResponse struct {
	Organizations []Organization `json:"organizations"`
}

// ListOrganizations fetches the user's organizations.
func (c *Client) ListOrganizations() ([]Organization, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/api/v1/me/organizations", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list organizations failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var orgsResp ListOrganizationsResponse
	if err := json.Unmarshal(respBody, &orgsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return orgsResp.Organizations, nil
}

// ListNetworksResponse is returned from listing networks.
type ListNetworksResponse struct {
	Networks []Network `json:"networks"`
}

// ListNetworks fetches the networks in an organization.
func (c *Client) ListNetworks(orgID string) ([]Network, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/api/v1/organizations/"+orgID+"/networks", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list networks failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var networksResp ListNetworksResponse
	if err := json.Unmarshal(respBody, &networksResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return networksResp.Networks, nil
}

// EnrollRequestV2 is sent for enrollment with organization/network context.
type EnrollRequestV2 struct {
	NodeID         string            `json:"node_id"`
	Token          string            `json:"token,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Roles          []string          `json:"roles,omitempty"` // ["olt", "bng"] - equipment types to manage
	OrganizationID string            `json:"organization_id"`
	NetworkID      string            `json:"network_id"`
	NetworkSlug    string            `json:"network_slug"` // K8s namespace
}

// EnrollV2 registers a node with organization/network context.
func (c *Client) EnrollV2(req *EnrollRequestV2) (*EnrollResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/nodes/enroll", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("enrollment failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var enrollResp EnrollResponse
	if err := json.Unmarshal(respBody, &enrollResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &enrollResp, nil
}

// EventSeverity represents the severity level of an event.
type EventSeverity string

const (
	SeverityCritical EventSeverity = "critical"
	SeverityWarning  EventSeverity = "warning"
	SeverityInfo     EventSeverity = "info"
)

// EmitEventRequest is sent to create a network event.
type EmitEventRequest struct {
	NodeID    string                 `json:"nodeId"`
	EventType string                 `json:"eventType"`
	Severity  EventSeverity          `json:"severity"`
	Content   string                 `json:"content"`
	EntityID  string                 `json:"entityId,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// EmitEventResponse is returned after creating an event.
type EmitEventResponse struct {
	Success bool `json:"success"`
	Event   struct {
		ID        string `json:"id"`
		NetworkID string `json:"networkId"`
		EventType string `json:"eventType"`
		Severity  string `json:"severity"`
		Content   string `json:"content"`
		CreatedAt string `json:"createdAt"`
	} `json:"event,omitempty"`
	Error string `json:"error,omitempty"`
}

// EmitEvent sends a network event to the control plane.
// This triggers push notifications and real-time broadcasts to users.
// Uses agent API key (na_) for per-agent rate limiting (60 req/min sustained, 120 burst).
func (c *Client) EmitEvent(req *EmitEventRequest) (*EmitEventResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/network-events", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Use agent API key (na_) for per-agent rate limiting
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for server signals (e.g., key rotation required)
	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var eventResp EmitEventResponse
	if err := json.Unmarshal(respBody, &eventResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return &eventResp, fmt.Errorf("emit event failed (HTTP %d): %s", resp.StatusCode, eventResp.Error)
	}

	return &eventResp, nil
}

// Common event types for convenience
const (
	EventTypeAgentConnected       = "agent_connected"
	EventTypeAgentDisconnected    = "agent_disconnected"
	EventTypeAgentHeartbeatMissed = "agent_heartbeat_missed"
	EventTypeEntityStatusChanged  = "entity_status_changed"
	EventTypeEntityOffline        = "entity_offline"
	EventTypeEntityOnline         = "entity_online"
	EventTypeEntityDegraded       = "entity_degraded"
	EventTypeHighCPUUsage         = "high_cpu_usage"
	EventTypeHighMemoryUsage      = "high_memory_usage"
	EventTypeHighBandwidthUsage   = "high_bandwidth_usage"
	EventTypePacketLossDetected   = "packet_loss_detected"
	EventTypeConfigChanged        = "config_changed"
	EventTypeConfigApplied        = "config_applied"
	EventTypeConfigFailed         = "config_failed"
)

// KeyRotateResponse is returned from the key rotation endpoint.
type KeyRotateResponse struct {
	Success           bool   `json:"success"`
	Message           string `json:"message,omitempty"`
	AgentAPIKey       string `json:"agent_api_key,omitempty"`
	AgentAPIKeyPrefix string `json:"agent_api_key_prefix,omitempty"`
	AgentID           string `json:"agent_id,omitempty"`
	OldKeyValidUntil  string `json:"old_key_valid_until,omitempty"`
}

// NewClientWithAgentKey creates a client with an agent-specific API key (na_ prefix).
// This provides per-agent rate limiting instead of IP-based limiting.
func NewClientWithAgentKey(baseURL, agentAPIKey, agentID string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   agentAPIKey,
		agentID: agentID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetAgentID sets the agent ID for key rotation purposes.
func (c *Client) SetAgentID(agentID string) {
	c.agentID = agentID
}

// GetAgentID returns the agent ID.
func (c *Client) GetAgentID() string {
	return c.agentID
}

// NeedsKeyRotation returns true if the server has signaled that key rotation is required.
func (c *Client) NeedsKeyRotation() bool {
	return c.keyRotationNeeded
}

// ClearKeyRotationFlag clears the key rotation flag after a successful rotation.
func (c *Client) ClearKeyRotationFlag() {
	c.keyRotationNeeded = false
}

// checkResponseHeaders inspects response headers for server signals.
// Sets keyRotationNeeded if the server sends X-Key-Rotation-Required: true.
func (c *Client) checkResponseHeaders(resp *http.Response) {
	if resp.Header.Get("X-Key-Rotation-Required") == "true" {
		c.keyRotationNeeded = true
	}
}

// RotateAgentKey requests a new agent API key from the server.
// This should be called when NeedsKeyRotation() returns true.
func (c *Client) RotateAgentKey() (*KeyRotateResponse, error) {
	if c.agentID == "" {
		return nil, fmt.Errorf("agent ID not set - cannot rotate key")
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/agents/"+c.agentID+"/keys/rotate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("key rotation failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var rotateResp KeyRotateResponse
	if err := json.Unmarshal(respBody, &rotateResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Clear the rotation flag after successful rotation
	c.keyRotationNeeded = false

	return &rotateResp, nil
}

// UpdateToken updates the client's API token (for use after key rotation).
func (c *Client) UpdateToken(newToken string) {
	c.token = newToken
}

// PostJSON makes a POST request with JSON payload and returns the response.
// This is a generic helper for API calls that don't need special response parsing.
func (c *Client) PostJSON(ctx interface{}, path string, jsonData []byte) (*http.Response, error) {
	httpReq, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	c.checkResponseHeaders(resp)
	return resp, nil
}

// OLTConfig represents an OLT configuration from the control plane.
type OLTConfig struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Vendor    string             `json:"vendor"`
	Model     string             `json:"model"`
	Address   string             `json:"address"`
	Protocols OLTProtocols       `json:"protocols"`
	Polling   OLTPollingConfig   `json:"polling"`
	Discovery OLTDiscoveryConfig `json:"discovery"`
}

// OLTProtocols contains protocol configurations for OLT access.
// Supports both legacy format (snmp/ssh fields) and new multi-protocol format (primary + protocol map).
type OLTProtocols struct {
	// New multi-protocol format
	Primary string `json:"primary,omitempty"` // Primary protocol: cli, snmp, netconf, gnmi, rest

	// Protocol-specific configurations (new format)
	CLI     *OLTCLIConfig     `json:"cli,omitempty"`
	NETCONF *OLTNETCONFConfig `json:"netconf,omitempty"`
	GNMI    *OLTGNMIConfig    `json:"gnmi,omitempty"`
	REST    *OLTRESTConfig    `json:"rest,omitempty"`

	// Legacy format (still supported)
	SNMP OLTSNMPConfig `json:"snmp"`
	SSH  OLTSSHConfig  `json:"ssh"`
}

// OLTCLIConfig contains CLI/SSH configuration for OLT access.
type OLTCLIConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// OLTNETCONFConfig contains NETCONF configuration.
type OLTNETCONFConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// OLTGNMIConfig contains gNMI configuration.
type OLTGNMIConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	TLSEnabled        bool   `json:"tlsEnabled,omitempty"`
}

// OLTRESTConfig contains REST API configuration.
type OLTRESTConfig struct {
	Enabled           bool   `json:"enabled"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	TLSEnabled        bool   `json:"tlsEnabled,omitempty"`
	BasePath          string `json:"basePath,omitempty"`
}

// OLTSNMPConfig contains SNMP configuration.
type OLTSNMPConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Community string `json:"community"`
	Version   string `json:"version"`
}

// OLTSSHConfig contains SSH configuration.
type OLTSSHConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// OLTPollingConfig contains polling configuration.
type OLTPollingConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"`
	Metrics  []string `json:"metrics"`
}

// OLTDiscoveryConfig contains discovery configuration.
type OLTDiscoveryConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"`
	Protocol string   `json:"protocol"`
	PONPorts []string `json:"ponPorts"`
}

// GetPrimaryProtocol returns the primary protocol for this OLT.
// Falls back to "cli" if not specified, or checks legacy SSH/SNMP config.
func (p *OLTProtocols) GetPrimaryProtocol() string {
	if p.Primary != "" {
		return p.Primary
	}
	// Check new format CLI first
	if p.CLI != nil && p.CLI.Enabled {
		return "cli"
	}
	// Legacy: check SSH
	if p.SSH.Enabled {
		return "cli"
	}
	// Check SNMP
	if p.SNMP.Enabled {
		return "snmp"
	}
	// Check other protocols
	if p.NETCONF != nil && p.NETCONF.Enabled {
		return "netconf"
	}
	if p.GNMI != nil && p.GNMI.Enabled {
		return "gnmi"
	}
	if p.REST != nil && p.REST.Enabled {
		return "rest"
	}
	// Default to cli
	return "cli"
}

// HasProtocol checks if a specific protocol is enabled.
func (p *OLTProtocols) HasProtocol(protocol string) bool {
	switch protocol {
	case "cli", "ssh":
		// Check new format CLI
		if p.CLI != nil && p.CLI.Enabled {
			return true
		}
		// Legacy: check SSH
		return p.SSH.Enabled
	case "snmp":
		return p.SNMP.Enabled
	case "netconf":
		return p.NETCONF != nil && p.NETCONF.Enabled
	case "gnmi":
		return p.GNMI != nil && p.GNMI.Enabled
	case "rest":
		return p.REST != nil && p.REST.Enabled
	default:
		return false
	}
}

// GetEnabledProtocols returns a list of all enabled protocol names.
func (p *OLTProtocols) GetEnabledProtocols() []string {
	var enabled []string
	if p.CLI != nil && p.CLI.Enabled {
		enabled = append(enabled, "cli")
	} else if p.SSH.Enabled {
		// Legacy SSH maps to CLI
		enabled = append(enabled, "cli")
	}
	if p.SNMP.Enabled {
		enabled = append(enabled, "snmp")
	}
	if p.NETCONF != nil && p.NETCONF.Enabled {
		enabled = append(enabled, "netconf")
	}
	if p.GNMI != nil && p.GNMI.Enabled {
		enabled = append(enabled, "gnmi")
	}
	if p.REST != nil && p.REST.Enabled {
		enabled = append(enabled, "rest")
	}
	return enabled
}

// NormalizeLegacyFormat converts legacy SSH config to new CLI format if needed.
// This ensures backward compatibility while using the new protocol model.
func (p *OLTProtocols) NormalizeLegacyFormat() {
	// If SSH is enabled but CLI is not set, convert SSH to CLI
	if p.SSH.Enabled && p.CLI == nil {
		p.CLI = &OLTCLIConfig{
			Enabled:  true,
			Port:     p.SSH.Port,
			Username: p.SSH.Username,
			Password: p.SSH.Password,
		}
	}
	// Set default primary if not specified
	if p.Primary == "" {
		p.Primary = p.GetPrimaryProtocol()
	}
}

// VendorCapability defines the required protocols for a vendor.
type VendorCapability struct {
	ConfigMethod    string // Required protocol for configuration (cli, netconf, etc.)
	TelemetryMethod string // Required protocol for telemetry (snmp, gnmi, etc.)
}

// VendorCapabilityMatrix maps vendor names to their capability requirements.
// This is used to validate that required protocols are enabled for each vendor.
var VendorCapabilityMatrix = map[string]VendorCapability{
	"huawei": {ConfigMethod: "cli", TelemetryMethod: "snmp"},
	"zte":    {ConfigMethod: "cli", TelemetryMethod: "snmp"},
	"vsol":   {ConfigMethod: "cli", TelemetryMethod: "snmp"},
	"cdata":  {ConfigMethod: "cli", TelemetryMethod: "snmp"},
	"nokia":  {ConfigMethod: "netconf", TelemetryMethod: "gnmi"},
	"adtran": {ConfigMethod: "netconf", TelemetryMethod: "snmp"},
}

// ValidateProtocolsForVendor validates that required protocols are enabled for the vendor.
// Returns an error if a required protocol is not enabled.
func (cfg *OLTConfig) ValidateProtocolsForVendor() error {
	vendor := cfg.Vendor
	if vendor == "" {
		return nil // No vendor specified, skip validation
	}

	cap, ok := VendorCapabilityMatrix[vendor]
	if !ok {
		// Unknown vendor, allow any protocol configuration
		return nil
	}

	// Validate ConfigMethod is enabled
	if cap.ConfigMethod != "" && !cfg.Protocols.HasProtocol(cap.ConfigMethod) {
		return fmt.Errorf("vendor %s requires config protocol %s but it is not enabled", vendor, cap.ConfigMethod)
	}

	// Validate TelemetryMethod is enabled
	if cap.TelemetryMethod != "" && !cfg.Protocols.HasProtocol(cap.TelemetryMethod) {
		return fmt.Errorf("vendor %s requires telemetry protocol %s but it is not enabled", vendor, cap.TelemetryMethod)
	}

	return nil
}

// AgentConfigResponse is the response from the agent config endpoint.
type AgentConfigResponse struct {
	NodeID  string      `json:"nodeId"`
	Version int         `json:"version"`
	OLTs    []OLTConfig `json:"olts"`
}

// GetOLTConfig retrieves OLT configuration from the control plane.
func (c *Client) GetOLTConfig(nodeID string) (*AgentConfigResponse, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/api/v1/nodes/"+nodeID+"/config", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get OLT config failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var config AgentConfigResponse
	if err := json.Unmarshal(respBody, &config); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &config, nil
}

// ONUData represents ONU data to be pushed to the control plane.
type ONUData struct {
	Serial          string  `json:"serialNumber"`
	PONPort         string  `json:"ponPort"`
	ONUID           int     `json:"onuId,omitempty"`
	Status          string  `json:"status"`
	OperState       string  `json:"operState,omitempty"`
	Distance        int     `json:"distance,omitempty"`
	RxPower         float64 `json:"rxPower,omitempty"`
	TxPower         float64 `json:"txPower,omitempty"`
	Model           string  `json:"model,omitempty"`
	SoftwareVersion string  `json:"softwareVersion,omitempty"`
}

// PushONUsRequest is the request body for pushing ONUs.
type PushONUsRequest struct {
	ONUs []ONUData `json:"onus"`
}

// PushONUsResponse is the response from pushing ONUs.
type PushONUsResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message,omitempty"`
	Created     int    `json:"created"`
	Updated     int    `json:"updated"`
	Unchanged   int    `json:"unchanged"`
	OnlineCount int    `json:"onlineCount"`
}

// PushONUs pushes ONU data to the control plane.
func (c *Client) PushONUs(oltID string, onus []ONUData) (*PushONUsResponse, error) {
	reqBody := PushONUsRequest{ONUs: onus}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/equipment/"+oltID+"/onus", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("push ONUs failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var pushResp PushONUsResponse
	if err := json.Unmarshal(respBody, &pushResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pushResp, nil
}
