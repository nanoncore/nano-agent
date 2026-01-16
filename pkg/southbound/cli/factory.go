package cli

import (
	"fmt"
	"strings"
)

// DriverFactory creates CLIDriver instances based on vendor and model.
type DriverFactory struct {
	// Registry of driver constructors by vendor name
	constructors map[string]DriverConstructor

	// Registry of model-specific capabilities by "vendor:model" key
	modelCapabilities map[string]*VendorCapabilities
}

// DriverConstructor is a function that creates a new CLIDriver instance.
type DriverConstructor func(config CLIConfig, model string) (CLIDriver, error)

// DefaultFactory is the global default driver factory instance.
var DefaultFactory = NewDriverFactory()

// NewDriverFactory creates a new DriverFactory instance.
func NewDriverFactory() *DriverFactory {
	return &DriverFactory{
		constructors:      make(map[string]DriverConstructor),
		modelCapabilities: make(map[string]*VendorCapabilities),
	}
}

// RegisterDriver registers a driver constructor for a vendor.
// The constructor will be called with the config and model name.
func (f *DriverFactory) RegisterDriver(vendor string, constructor DriverConstructor) {
	f.constructors[strings.ToLower(vendor)] = constructor
}

// RegisterModelCapabilities registers capabilities for a specific vendor/model combination.
func (f *DriverFactory) RegisterModelCapabilities(vendor, model string, caps *VendorCapabilities) {
	key := strings.ToLower(fmt.Sprintf("%s:%s", vendor, model))
	f.modelCapabilities[key] = caps
}

// CreateDriver creates a new CLIDriver for the specified vendor and model.
// If a model is specified, model-specific optimizations may be applied.
func (f *DriverFactory) CreateDriver(config CLIConfig, model string) (CLIDriver, error) {
	vendor := strings.ToLower(config.Vendor)

	constructor, ok := f.constructors[vendor]
	if !ok {
		return nil, fmt.Errorf("unsupported vendor: %s", config.Vendor)
	}

	return constructor(config, model)
}

// GetCapabilities returns the capabilities for a vendor/model combination.
// If the model is empty or not found, vendor-level defaults are returned.
func (f *DriverFactory) GetCapabilities(vendor, model string) *VendorCapabilities {
	vendor = strings.ToLower(vendor)

	// Try exact vendor:model match first
	if model != "" {
		key := fmt.Sprintf("%s:%s", vendor, strings.ToLower(model))
		if caps, ok := f.modelCapabilities[key]; ok {
			return caps
		}
	}

	// Try vendor-only match
	key := fmt.Sprintf("%s:", vendor)
	if caps, ok := f.modelCapabilities[key]; ok {
		return caps
	}

	// Return default capabilities based on vendor
	return f.defaultCapabilities(vendor, model)
}

// defaultCapabilities returns default capabilities for known vendors.
func (f *DriverFactory) defaultCapabilities(vendor, model string) *VendorCapabilities {
	switch vendor {
	case "huawei":
		if model != "" && strings.HasPrefix(strings.ToUpper(model), "MA5800") {
			return HuaweiMA5800Capabilities()
		}
		if model != "" && strings.HasPrefix(strings.ToUpper(model), "MA5600") {
			return HuaweiMA5600TCapabilities()
		}
		return FullCapabilities(vendor, model)

	case "zte":
		if model != "" && strings.HasPrefix(strings.ToUpper(model), "C600") {
			return ZTEC600Capabilities()
		}
		if model != "" && strings.HasPrefix(strings.ToUpper(model), "C300") {
			return ZTEC300Capabilities()
		}
		return FullCapabilities(vendor, model)

	case "nokia":
		return NokiaISAMCapabilities()

	case "vsol":
		return VSOLCapabilities(model)

	case "cdata":
		return CDataCapabilities(model)

	case "fiberhome":
		return FiberHomeCapabilities(model)

	default:
		// Unknown vendor - return minimal capabilities
		return MinimalCapabilities(vendor, model)
	}
}

// SupportedVendors returns a list of registered vendor names.
func (f *DriverFactory) SupportedVendors() []string {
	vendors := make([]string, 0, len(f.constructors))
	for vendor := range f.constructors {
		vendors = append(vendors, vendor)
	}
	return vendors
}

// IsVendorSupported checks if a vendor is registered in the factory.
func (f *DriverFactory) IsVendorSupported(vendor string) bool {
	_, ok := f.constructors[strings.ToLower(vendor)]
	return ok
}

// RegisterDriver registers a driver constructor in the default factory.
func RegisterDriver(vendor string, constructor DriverConstructor) {
	DefaultFactory.RegisterDriver(vendor, constructor)
}

// RegisterModelCapabilities registers model capabilities in the default factory.
func RegisterModelCapabilities(vendor, model string, caps *VendorCapabilities) {
	DefaultFactory.RegisterModelCapabilities(vendor, model, caps)
}

// CreateDriver creates a driver using the default factory.
func CreateDriver(config CLIConfig, model string) (CLIDriver, error) {
	return DefaultFactory.CreateDriver(config, model)
}

// GetCapabilities returns capabilities from the default factory.
func GetCapabilities(vendor, model string) *VendorCapabilities {
	return DefaultFactory.GetCapabilities(vendor, model)
}

// SupportedVendors returns supported vendors from the default factory.
func SupportedVendors() []string {
	return DefaultFactory.SupportedVendors()
}

// IsVendorSupported checks vendor support in the default factory.
func IsVendorSupported(vendor string) bool {
	return DefaultFactory.IsVendorSupported(vendor)
}
