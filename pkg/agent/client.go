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
type OLTProtocols struct {
	SNMP SNMPConfig `json:"snmp"`
	SSH  SSHConfig  `json:"ssh"`
}

// SNMPConfig contains SNMP configuration.
type SNMPConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Community string `json:"community"`
	Version   string `json:"version"`
}

// SSHConfig contains SSH configuration.
type SSHConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// OLTPollingConfig contains polling configuration.
type OLTPollingConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"` // seconds
	Metrics  []string `json:"metrics"`
}

// OLTDiscoveryConfig contains discovery configuration.
type OLTDiscoveryConfig struct {
	Enabled  bool     `json:"enabled"`
	Interval int      `json:"interval"` // seconds
	Protocol string   `json:"protocol"`
	PONPorts []string `json:"ponPorts"`
}

// AgentConfigResponse represents the full configuration response from the control plane.
type AgentConfigResponse struct {
	NodeID          string           `json:"nodeId"`
	Version         int              `json:"version"`
	OLTs            []OLTConfig      `json:"olts"`
	PendingProbes   []PendingProbe   `json:"pendingProbes,omitempty"`
	PendingCommands []PendingCommand `json:"pendingCommands,omitempty"`
}

// PendingCommand represents a command queued by the control plane for execution.
type PendingCommand struct {
	ID          string                 `json:"id"`
	EquipmentID string                 `json:"equipmentId"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
}

// PendingProbe represents a probe request queued by the control plane.
type PendingProbe struct {
	ID       string `json:"id"`
	OLTID    string `json:"oltId"`
	Detailed bool   `json:"detailed"`
}

// AckProbeRequest is sent to acknowledge a completed probe.
type AckProbeRequest struct {
	ProbeID string `json:"probeId"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// AckProbe acknowledges a completed probe to the control plane.
func (c *Client) AckProbe(nodeID string, req *AckProbeRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/nodes/"+nodeID+"/probes/ack", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ack probe failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetOLTConfig retrieves the typed OLT configuration from the control plane.
func (c *Client) GetOLTConfig(nodeID string) (*AgentConfigResponse, error) {
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
	ONUID           int     `json:"onuId"`
	Status          string  `json:"status"`
	Distance        int     `json:"distance,omitempty"`
	RxPower         float64 `json:"rxPower,omitempty"`
	TxPower         float64 `json:"txPower,omitempty"`
	Model           string  `json:"model,omitempty"`
	SoftwareVersion string  `json:"softwareVersion,omitempty"`

	// Thermal & Power (from detailed poll)
	Temperature float64 `json:"temperature,omitempty"` // °C
	Voltage     float64 `json:"voltage,omitempty"`     // V
	BiasCurrent float64 `json:"biasCurrent,omitempty"` // mA

	// Traffic Stats (from detailed poll)
	BytesUp       uint64 `json:"bytesUp,omitempty"`
	BytesDown     uint64 `json:"bytesDown,omitempty"`
	PacketsUp     uint64 `json:"packetsUp,omitempty"`
	PacketsDown   uint64 `json:"packetsDown,omitempty"`
	InputRateBps  uint64 `json:"inputRateBps,omitempty"`
	OutputRateBps uint64 `json:"outputRateBps,omitempty"`

	// Vendor detection
	Vendor string `json:"vendor,omitempty"`

	// Profile and VLAN (from ONU list/detail)
	ONUProfile    string `json:"onuProfile,omitempty"`
	LineProfile    string `json:"lineProfile,omitempty"`
	ServiceProfile string `json:"serviceProfile,omitempty"`
	VLAN           int    `json:"vlan,omitempty"`
}

// PushONUsRequest is the request body for pushing ONUs to the control plane.
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

// PushONUs sends discovered ONUs to the control plane.
// This calls POST /api/v1/equipment/{oltId}/onus
func (c *Client) PushONUs(oltID string, onus []ONUData) (*PushONUsResponse, error) {
	req := &PushONUsRequest{ONUs: onus}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/equipment/"+oltID+"/onus", bytes.NewReader(body))
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

	// Check for server signals
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

// PushSingleONU pushes a single ONU update immediately after command execution.
// This is a convenience wrapper around PushONUs for immediate post-command updates.
func (c *Client) PushSingleONU(oltID string, onu ONUData) (*PushONUsResponse, error) {
	return c.PushONUs(oltID, []ONUData{onu})
}

// TelemetryData represents telemetry data to be pushed to the control plane.
type TelemetryData struct {
	CPUPercent    float64 `json:"cpuPercent,omitempty"`
	MemoryPercent float64 `json:"memoryPercent,omitempty"`
	Temperature   float64 `json:"temperature,omitempty"`
	Uptime        int64   `json:"uptime,omitempty"`
	IsReachable   bool    `json:"isReachable"`
	IsHealthy     bool    `json:"isHealthy"`
	Firmware      string  `json:"firmware,omitempty"`
	SerialNumber  string  `json:"serialNumber,omitempty"`
}

// PushTelemetryResponse is the response from pushing telemetry.
type PushTelemetryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// PushTelemetry sends OLT telemetry data to the control plane.
// This calls POST /api/v1/equipment/{oltId}/telemetry
func (c *Client) PushTelemetry(oltID string, telemetry *TelemetryData) (*PushTelemetryResponse, error) {
	body, err := json.Marshal(telemetry)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/equipment/"+oltID+"/telemetry", bytes.NewReader(body))
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

	// Check for server signals
	c.checkResponseHeaders(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("push telemetry failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var pushResp PushTelemetryResponse
	if err := json.Unmarshal(respBody, &pushResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pushResp, nil
}

// MetricSample represents a single metric data point for time-series storage.
type MetricSample struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"` // Unix milliseconds
	Labels    map[string]string `json:"labels"`
}

// MetricsBatch is a batch of metrics to push to the control plane.
type MetricsBatch struct {
	Metrics []MetricSample `json:"metrics"`
}

// PushMetricsResponse is the response from pushing metrics.
type PushMetricsResponse struct {
	Success bool   `json:"success"`
	Count   int    `json:"count"`
	Message string `json:"message,omitempty"`
}

// PushMetrics sends a batch of metrics to the control plane for time-series storage.
// This calls POST /api/v1/agent/metrics
func (c *Client) PushMetrics(batch *MetricsBatch) (*PushMetricsResponse, error) {
	if batch == nil || len(batch.Metrics) == 0 {
		return &PushMetricsResponse{Success: true, Count: 0}, nil
	}

	body, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/agent/metrics", bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("push metrics failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var pushResp PushMetricsResponse
	if err := json.Unmarshal(respBody, &pushResp); err != nil {
		// If response is not JSON, assume success
		return &PushMetricsResponse{Success: true, Count: len(batch.Metrics)}, nil
	}

	return &pushResp, nil
}

// CommandAckResponse is the response from acknowledging a command.
type CommandAckResponse struct {
	Success   bool   `json:"success"`
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// AckCommand acknowledges receipt of a command and marks it as in_progress.
// This should be called when the agent starts executing a command.
func (c *Client) AckCommand(commandID string) (*CommandAckResponse, error) {
	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/commands/"+commandID+"/ack", nil)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ack command failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var ackResp CommandAckResponse
	if err := json.Unmarshal(respBody, &ackResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &ackResp, nil
}

// CommandResultRequest is sent to push command execution results.
type CommandResultRequest struct {
	Success    bool                   `json:"success"`
	Result     map[string]interface{} `json:"result,omitempty"`
	Error      string                 `json:"error,omitempty"`
	PreState   map[string]interface{} `json:"preState,omitempty"`
	PostState  map[string]interface{} `json:"postState,omitempty"`
	Verified   bool                   `json:"verified,omitempty"`
	DurationMs int64                  `json:"durationMs,omitempty"`
}

// CommandResultResponse is the response from pushing command results.
type CommandResultResponse struct {
	Success   bool   `json:"success"`
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// PushCommandResult sends the result of a command execution to the control plane.
func (c *Client) PushCommandResult(commandID string, req *CommandResultRequest) (*CommandResultResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/commands/"+commandID+"/result", bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("push command result failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var resultResp CommandResultResponse
	if err := json.Unmarshal(respBody, &resultResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resultResp, nil
}
