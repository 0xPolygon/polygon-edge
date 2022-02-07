package command

import "github.com/0xPolygon/polygon-edge/server"

const (
	DefaultGenesisFileName = "genesis.json"
	DefaultChainName       = "polygon-edge"
	DefaultChainID         = 100
	DefaultPremineBalance  = "0x3635C9ADC5DEA00000" // 1000 ETH
	DefaultConsensus       = server.IBFTConsensus
	DefaultMaxSlots        = 4096
	DefaultGenesisGasUsed  = 458752  // 0x70000
	DefaultGenesisGasLimit = 5242880 // 0x500000
)

const (
	JSONOutputFlag  = "json"
	GRPCAddressFlag = "grpc-address"
	JSONRPCFlag     = "jsonrpc"
)
