module github.com/nanoncore/nano-agent

go 1.24.0

require (
	github.com/google/goexpect v0.0.0-20210430020637-ab937bf7fd6f
	github.com/gosnmp/gosnmp v1.42.1
	github.com/nanoncore/nano-southbound v0.3.8-0.20260109151646-42a27d7f4488
	github.com/spf13/cobra v1.10.1
	golang.org/x/crypto v0.45.0
)

require (
	github.com/google/goterm v0.0.0-20190703233501-fc88cf888a3f // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/openconfig/gnmi v0.14.1 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

// Local development: uncomment the line below to use local nano-southbound
// replace github.com/nanoncore/nano-southbound => ../nano-southbound
