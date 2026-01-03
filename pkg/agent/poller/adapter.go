package poller

import (
	"github.com/nanoncore/nano-agent/pkg/agent"
)

// ClientAdapter adapts the agent.Client to implement the ONUPusher interface.
type ClientAdapter struct {
	client *agent.Client
}

// NewClientAdapter creates a new adapter wrapping an agent.Client.
func NewClientAdapter(client *agent.Client) *ClientAdapter {
	return &ClientAdapter{client: client}
}

// PushONUs implements the ONUPusher interface.
func (a *ClientAdapter) PushONUs(oltID string, onus []ONUData) (*PushONUsResponse, error) {
	// Convert poller.ONUData to agent.ONUData
	agentONUs := make([]agent.ONUData, len(onus))
	for i, onu := range onus {
		agentONUs[i] = agent.ONUData{
			Serial:          onu.Serial,
			PONPort:         onu.PONPort,
			ONUID:           onu.ONUID,
			Status:          onu.Status,
			Distance:        onu.Distance,
			RxPower:         onu.RxPower,
			TxPower:         onu.TxPower,
			Model:           onu.Model,
			SoftwareVersion: onu.SoftwareVersion,
		}
	}

	// Call the agent client
	resp, err := a.client.PushONUs(oltID, agentONUs)
	if err != nil {
		return nil, err
	}

	// Convert response
	return &PushONUsResponse{
		Success:     resp.Success,
		Message:     resp.Message,
		Created:     resp.Created,
		Updated:     resp.Updated,
		Unchanged:   resp.Unchanged,
		OnlineCount: resp.OnlineCount,
	}, nil
}

// ConvertOLTConfigs converts agent.OLTConfig to poller.OLTConfig.
// Supports both legacy format (snmp/ssh) and new multi-protocol format.
func ConvertOLTConfigs(agentConfigs []agent.OLTConfig) []OLTConfig {
	configs := make([]OLTConfig, len(agentConfigs))
	for i, cfg := range agentConfigs {
		// Normalize legacy format before conversion
		cfg.Protocols.NormalizeLegacyFormat()

		pollerConfig := OLTConfig{
			ID:      cfg.ID,
			Name:    cfg.Name,
			Vendor:  cfg.Vendor,
			Model:   cfg.Model,
			Address: cfg.Address,
			Protocols: OLTProtocols{
				Primary: cfg.Protocols.Primary,
				// Legacy SNMP/SSH
				SNMP: SNMPConfig{
					Enabled:   cfg.Protocols.SNMP.Enabled,
					Port:      cfg.Protocols.SNMP.Port,
					Community: cfg.Protocols.SNMP.Community,
					Version:   cfg.Protocols.SNMP.Version,
				},
				SSH: SSHConfig{
					Enabled:  cfg.Protocols.SSH.Enabled,
					Port:     cfg.Protocols.SSH.Port,
					Username: cfg.Protocols.SSH.Username,
					Password: cfg.Protocols.SSH.Password,
				},
			},
			Polling: OLTPollingConfig{
				Enabled:  cfg.Polling.Enabled,
				Interval: cfg.Polling.Interval,
				Metrics:  cfg.Polling.Metrics,
			},
			Discovery: OLTDiscoveryConfig{
				Enabled:  cfg.Discovery.Enabled,
				Interval: cfg.Discovery.Interval,
				Protocol: cfg.Discovery.Protocol,
				PONPorts: cfg.Discovery.PONPorts,
			},
		}

		// Convert new multi-protocol configs
		if cfg.Protocols.CLI != nil {
			pollerConfig.Protocols.CLI = &CLIConfig{
				Enabled:           cfg.Protocols.CLI.Enabled,
				Port:              cfg.Protocols.CLI.Port,
				Username:          cfg.Protocols.CLI.Username,
				Password:          cfg.Protocols.CLI.Password,
				CredentialsSecret: cfg.Protocols.CLI.CredentialsSecret,
			}
		}
		if cfg.Protocols.NETCONF != nil {
			pollerConfig.Protocols.NETCONF = &NETCONFConfig{
				Enabled:           cfg.Protocols.NETCONF.Enabled,
				Port:              cfg.Protocols.NETCONF.Port,
				Username:          cfg.Protocols.NETCONF.Username,
				Password:          cfg.Protocols.NETCONF.Password,
				CredentialsSecret: cfg.Protocols.NETCONF.CredentialsSecret,
			}
		}
		if cfg.Protocols.GNMI != nil {
			pollerConfig.Protocols.GNMI = &GNMIConfig{
				Enabled:           cfg.Protocols.GNMI.Enabled,
				Port:              cfg.Protocols.GNMI.Port,
				Username:          cfg.Protocols.GNMI.Username,
				Password:          cfg.Protocols.GNMI.Password,
				CredentialsSecret: cfg.Protocols.GNMI.CredentialsSecret,
				TLSEnabled:        cfg.Protocols.GNMI.TLSEnabled,
			}
		}
		if cfg.Protocols.REST != nil {
			pollerConfig.Protocols.REST = &RESTConfig{
				Enabled:           cfg.Protocols.REST.Enabled,
				Port:              cfg.Protocols.REST.Port,
				Username:          cfg.Protocols.REST.Username,
				Password:          cfg.Protocols.REST.Password,
				CredentialsSecret: cfg.Protocols.REST.CredentialsSecret,
				TLSEnabled:        cfg.Protocols.REST.TLSEnabled,
				BasePath:          cfg.Protocols.REST.BasePath,
			}
		}

		configs[i] = pollerConfig
	}
	return configs
}
