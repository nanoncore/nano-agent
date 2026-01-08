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

// PushTelemetry implements the TelemetryPusher interface.
func (a *ClientAdapter) PushTelemetry(oltID string, telemetry *TelemetryData) (*PushTelemetryResponse, error) {
	// Convert poller.TelemetryData to agent.TelemetryData
	agentTelemetry := &agent.TelemetryData{
		CPUPercent:    telemetry.CPUPercent,
		MemoryPercent: telemetry.MemoryPercent,
		Temperature:   telemetry.Temperature,
		Uptime:        telemetry.Uptime,
		IsReachable:   telemetry.IsReachable,
		IsHealthy:     telemetry.IsHealthy,
		Firmware:      telemetry.Firmware,
		SerialNumber:  telemetry.SerialNumber,
	}

	// Call the agent client
	resp, err := a.client.PushTelemetry(oltID, agentTelemetry)
	if err != nil {
		return nil, err
	}

	// Convert response
	return &PushTelemetryResponse{
		Success: resp.Success,
		Message: resp.Message,
	}, nil
}

// PushMetrics implements the MetricsPusher interface.
func (a *ClientAdapter) PushMetrics(batch *MetricsBatch) (*PushMetricsResponse, error) {
	if batch == nil || len(batch.Metrics) == 0 {
		return &PushMetricsResponse{Success: true, Count: 0}, nil
	}

	// Convert poller.MetricsBatch to agent.MetricsBatch
	agentBatch := &agent.MetricsBatch{
		Metrics: make([]agent.MetricSample, len(batch.Metrics)),
	}
	for i, m := range batch.Metrics {
		agentBatch.Metrics[i] = agent.MetricSample{
			Name:      m.Name,
			Value:     m.Value,
			Timestamp: m.Timestamp,
			Labels:    m.Labels,
		}
	}

	// Call the agent client
	resp, err := a.client.PushMetrics(agentBatch)
	if err != nil {
		return nil, err
	}

	// Convert response
	return &PushMetricsResponse{
		Success: resp.Success,
		Count:   resp.Count,
		Message: resp.Message,
	}, nil
}

// ConvertOLTConfigs converts agent.OLTConfig to poller.OLTConfig.
func ConvertOLTConfigs(agentConfigs []agent.OLTConfig) []OLTConfig {
	configs := make([]OLTConfig, len(agentConfigs))
	for i, cfg := range agentConfigs {
		configs[i] = OLTConfig{
			ID:      cfg.ID,
			Name:    cfg.Name,
			Vendor:  cfg.Vendor,
			Model:   cfg.Model,
			Address: cfg.Address,
			Protocols: OLTProtocols{
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
	}
	return configs
}
