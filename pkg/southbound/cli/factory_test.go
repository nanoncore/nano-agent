package cli

import (
	"context"
	"testing"
)

// MockDriver is a test double for CLIDriver
type MockDriver struct {
	vendor       string
	model        string
	capabilities *VendorCapabilities
}

func (m *MockDriver) Connect(ctx context.Context) error                              { return nil }
func (m *MockDriver) Close() error                                                   { return nil }
func (m *MockDriver) Execute(ctx context.Context, cmd string) (string, error)        { return "", nil }
func (m *MockDriver) Vendor() string                                                 { return m.vendor }
func (m *MockDriver) GetCapabilities() *VendorCapabilities                           { return m.capabilities }
func (m *MockDriver) AddONU(ctx context.Context, req *ONUProvisionRequest) error     { return nil }
func (m *MockDriver) DeleteONU(ctx context.Context, ponPort string, onuID int) error { return nil }
func (m *MockDriver) GetONUInfo(ctx context.Context, ponPort string, onuID int) (*ONUCLIInfo, error) {
	return nil, nil
}
func (m *MockDriver) RebootONU(ctx context.Context, ponPort string, onuID int) error { return nil }
func (m *MockDriver) ConfigureVLAN(ctx context.Context, config *VLANConfig) error    { return nil }
func (m *MockDriver) GetVLANConfig(ctx context.Context, ponPort string, onuID int) (*VLANConfig, error) {
	return nil, nil
}
func (m *MockDriver) AddVLANTranslation(ctx context.Context, ponPort string, onuID int, translation VLANTranslation) error {
	return nil
}
func (m *MockDriver) RemoveVLANTranslation(ctx context.Context, ponPort string, onuID int, customerVLAN int) error {
	return nil
}
func (m *MockDriver) ListVLANs(ctx context.Context) ([]VLANInfo, error)           { return nil, nil }
func (m *MockDriver) ListLineProfiles(ctx context.Context) ([]LineProfile, error) { return nil, nil }
func (m *MockDriver) GetLineProfile(ctx context.Context, profileID int) (*LineProfile, error) {
	return nil, nil
}
func (m *MockDriver) ListServiceProfiles(ctx context.Context) ([]ServiceProfile, error) {
	return nil, nil
}
func (m *MockDriver) GetServiceProfile(ctx context.Context, profileID int) (*ServiceProfile, error) {
	return nil, nil
}
func (m *MockDriver) ListTrafficProfiles(ctx context.Context) ([]TrafficProfile, error) {
	return nil, nil
}
func (m *MockDriver) AssignTrafficProfile(ctx context.Context, ponPort string, onuID int, profileID int) error {
	return nil
}
func (m *MockDriver) ListPONPorts(ctx context.Context) ([]PONPortInfo, error) { return nil, nil }
func (m *MockDriver) GetPONPortInfo(ctx context.Context, slot, port int) (*PONPortInfo, error) {
	return nil, nil
}
func (m *MockDriver) EnablePONPort(ctx context.Context, slot, port int) error  { return nil }
func (m *MockDriver) DisablePONPort(ctx context.Context, slot, port int) error { return nil }
func (m *MockDriver) SetPortDescription(ctx context.Context, slot, port int, description string) error {
	return nil
}
func (m *MockDriver) BatchProvision(ctx context.Context, req *BatchProvisionRequest) (*BatchResult, error) {
	return nil, nil
}
func (m *MockDriver) BatchConfigureVLAN(ctx context.Context, req *BatchVLANRequest) (*BatchResult, error) {
	return nil, nil
}
func (m *MockDriver) ExportConfig(ctx context.Context) (*ConfigExport, error) { return nil, nil }
func (m *MockDriver) GetONUDiagnostics(ctx context.Context, ponPort string, onuID int) (*ONUDiagnostics, error) {
	return nil, nil
}
func (m *MockDriver) GetONUCounters(ctx context.Context, ponPort string, onuID int) (*PerformanceCounters, error) {
	return nil, nil
}
func (m *MockDriver) ClearONUCounters(ctx context.Context, ponPort string, onuID int) error {
	return nil
}
func (m *MockDriver) GetOpticalDiagnostics(ctx context.Context, ponPort string, onuID int) (*OpticalDiagnostics, error) {
	return nil, nil
}
func (m *MockDriver) SaveConfig(ctx context.Context) error { return nil }

func TestDriverFactory_RegisterAndCreate(t *testing.T) {
	factory := NewDriverFactory()

	// Register a mock driver
	factory.RegisterDriver("test-vendor", func(config CLIConfig, model string) (CLIDriver, error) {
		return &MockDriver{
			vendor:       "test-vendor",
			model:        model,
			capabilities: FullCapabilities("test-vendor", model),
		}, nil
	})

	// Test driver creation
	config := CLIConfig{
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		Password: "password",
		Vendor:   "test-vendor",
	}

	driver, err := factory.CreateDriver(config, "TestModel")
	if err != nil {
		t.Fatalf("failed to create driver: %v", err)
	}

	if driver.Vendor() != "test-vendor" {
		t.Errorf("expected vendor 'test-vendor', got %q", driver.Vendor())
	}
}

func TestDriverFactory_UnsupportedVendor(t *testing.T) {
	factory := NewDriverFactory()

	config := CLIConfig{
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		Password: "password",
		Vendor:   "unknown-vendor",
	}

	_, err := factory.CreateDriver(config, "")
	if err == nil {
		t.Error("expected error for unsupported vendor")
	}
}

func TestDriverFactory_GetCapabilities(t *testing.T) {
	factory := NewDriverFactory()

	// Register model-specific capabilities
	customCaps := &VendorCapabilities{
		Vendor:            "custom",
		Model:             "X100",
		SupportsProvision: true,
		SupportsReboot:    false,
		MaxONUsPerPort:    256,
	}
	factory.RegisterModelCapabilities("custom", "X100", customCaps)

	// Test exact match
	caps := factory.GetCapabilities("custom", "X100")
	if caps.MaxONUsPerPort != 256 {
		t.Errorf("expected MaxONUsPerPort 256, got %d", caps.MaxONUsPerPort)
	}

	// Test default fallback for Huawei
	huaweiCaps := factory.GetCapabilities("huawei", "MA5800-X17")
	if huaweiCaps.MaxONUsPerPort != 128 {
		t.Errorf("expected MaxONUsPerPort 128 for Huawei, got %d", huaweiCaps.MaxONUsPerPort)
	}

	// Test unknown vendor
	unknownCaps := factory.GetCapabilities("unknown", "model")
	if unknownCaps.MaxONUsPerPort != 32 {
		t.Errorf("expected MaxONUsPerPort 32 for unknown vendor, got %d", unknownCaps.MaxONUsPerPort)
	}
}

func TestDriverFactory_SupportedVendors(t *testing.T) {
	factory := NewDriverFactory()

	factory.RegisterDriver("huawei", func(config CLIConfig, model string) (CLIDriver, error) {
		return &MockDriver{vendor: "huawei"}, nil
	})
	factory.RegisterDriver("zte", func(config CLIConfig, model string) (CLIDriver, error) {
		return &MockDriver{vendor: "zte"}, nil
	})

	vendors := factory.SupportedVendors()
	if len(vendors) != 2 {
		t.Errorf("expected 2 vendors, got %d", len(vendors))
	}

	// Check vendor support
	if !factory.IsVendorSupported("huawei") {
		t.Error("expected huawei to be supported")
	}
	if !factory.IsVendorSupported("zte") {
		t.Error("expected zte to be supported")
	}
	if factory.IsVendorSupported("nokia") {
		t.Error("expected nokia to not be supported")
	}
}

func TestDefaultFactory(t *testing.T) {
	// Test that the global default factory works
	caps := GetCapabilities("huawei", "MA5800")
	if caps == nil {
		t.Error("expected non-nil capabilities from default factory")
	}
	if caps.Vendor != "huawei" {
		t.Errorf("expected vendor 'huawei', got %q", caps.Vendor)
	}
}

func TestDriverFactory_CaseInsensitiveVendor(t *testing.T) {
	factory := NewDriverFactory()

	factory.RegisterDriver("HuaWei", func(config CLIConfig, model string) (CLIDriver, error) {
		return &MockDriver{vendor: "huawei"}, nil
	})

	// Test case insensitive lookup
	config := CLIConfig{
		Host:     "192.168.1.1",
		Port:     22,
		Username: "admin",
		Password: "password",
		Vendor:   "HUAWEI",
	}

	driver, err := factory.CreateDriver(config, "")
	if err != nil {
		t.Fatalf("expected case-insensitive vendor match, got error: %v", err)
	}
	if driver == nil {
		t.Error("expected non-nil driver")
	}
}
