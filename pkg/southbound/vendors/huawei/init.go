package huawei

import (
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

func init() {
	// Register Huawei driver constructor with the factory
	cli.RegisterDriver("huawei", func(config cli.CLIConfig, model string) (cli.CLIDriver, error) {
		return NewHuaweiCLIDriverWithModel(config, model), nil
	})

	// Register model-specific capabilities
	cli.RegisterModelCapabilities("huawei", "MA5800", cli.HuaweiMA5800Capabilities())
	cli.RegisterModelCapabilities("huawei", "MA5800-X2", cli.HuaweiMA5800Capabilities())
	cli.RegisterModelCapabilities("huawei", "MA5800-X7", cli.HuaweiMA5800Capabilities())
	cli.RegisterModelCapabilities("huawei", "MA5800-X15", cli.HuaweiMA5800Capabilities())
	cli.RegisterModelCapabilities("huawei", "MA5800-X17", cli.HuaweiMA5800Capabilities())
	cli.RegisterModelCapabilities("huawei", "MA5600T", cli.HuaweiMA5600TCapabilities())
	cli.RegisterModelCapabilities("huawei", "MA5608T", cli.HuaweiMA5600TCapabilities())
	cli.RegisterModelCapabilities("huawei", "MA5683T", cli.HuaweiMA5600TCapabilities())
	cli.RegisterModelCapabilities("huawei", "", cli.FullCapabilities("huawei", "")) // Default
}
