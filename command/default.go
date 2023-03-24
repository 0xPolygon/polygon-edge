package command

import (
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/umbracle/ethgo"
)

const (
	DefaultGenesisFileName = "genesis.json"
	DefaultChainName       = "polygon-edge"
	DefaultChainID         = 100
	DefaultConsensus       = server.PolyBFTConsensus
	DefaultGenesisGasUsed  = 458752  // 0x70000
	DefaultGenesisGasLimit = 5242880 // 0x500000
)

var (
	DefaultStake          = ethgo.Ether(1e6)
	DefaultPremineBalance = ethgo.Ether(1e6)
)

const (
	JSONOutputFlag  = "json"
	GRPCAddressFlag = "grpc-address"
	JSONRPCFlag     = "jsonrpc"
)

// GRPCAddressFlagLEGACY Legacy flag that needs to be present to preserve backwards
// compatibility with running clients
const (
	GRPCAddressFlagLEGACY = "grpc"
)
