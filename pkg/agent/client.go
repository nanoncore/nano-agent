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
