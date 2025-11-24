package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultConfigDir  = "/etc/nano-agent"
	DefaultConfigFile = "config.json"
	DefaultStateFile  = "state.json"
)

// Config holds the agent configuration persisted after enrollment.
type Config struct {
	APIURL   string            `json:"api_url"`
	NodeID   string            `json:"node_id"`
	Labels   map[string]string `json:"labels,omitempty"`
	CertFile string            `json:"cert_file,omitempty"`
	KeyFile  string            `json:"key_file,omitempty"`
	CAFile   string            `json:"ca_file,omitempty"`
}

// State holds runtime state such as enrollment status.
type State struct {
	Enrolled     bool   `json:"enrolled"`
	EnrolledAt   string `json:"enrolled_at,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
}

// LoadConfig reads the agent config from disk.
func LoadConfig(configDir string) (*Config, error) {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	path := filepath.Join(configDir, DefaultConfigFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent not enrolled (config not found at %s)", path)
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig writes the agent config to disk.
func SaveConfig(configDir string, cfg *Config) error {
	if configDir == "" {
		configDir = DefaultConfigDir
	}

	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, DefaultConfigFile)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// LoadState reads the agent state from disk.
func LoadState(configDir string) (*State, error) {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	path := filepath.Join(configDir, DefaultStateFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Enrolled: false}, nil
		}
		return nil, fmt.Errorf("failed to read state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}

	return &state, nil
}

// SaveState writes the agent state to disk.
func SaveState(configDir string, state *State) error {
	if configDir == "" {
		configDir = DefaultConfigDir
	}

	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, DefaultStateFile)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	return nil
}
