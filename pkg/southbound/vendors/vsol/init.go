package vsol

import (
	"github.com/nanoncore/nano-agent/pkg/southbound/cli"
)

func init() {
	// Register V-SOL driver constructor with the factory
	cli.RegisterDriver("vsol", func(config cli.CLIConfig, model string) (cli.CLIDriver, error) {
		return NewVSOLCLIDriverWithModel(config, model), nil
	})

	// Register model-specific capabilities
	cli.RegisterModelCapabilities("vsol", "V1600D", cli.VSOLCapabilities("V1600D"))
	cli.RegisterModelCapabilities("vsol", "V1600G", cli.VSOLCapabilities("V1600G"))
	cli.RegisterModelCapabilities("vsol", "V1600G4", cli.VSOLCapabilities("V1600G4"))
	cli.RegisterModelCapabilities("vsol", "V1600D4", cli.VSOLCapabilities("V1600D4"))
	cli.RegisterModelCapabilities("vsol", "", cli.VSOLCapabilities("")) // Default
}
