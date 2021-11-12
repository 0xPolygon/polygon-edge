package bridge

import (
	"github.com/ChainSafe/ChainBridge/chains/ethereum"
	"github.com/ChainSafe/chainbridge-utils/core"
	"github.com/ChainSafe/chainbridge-utils/msg"
	log "github.com/ChainSafe/log15"
)

type Chain struct {
	Name           string            // Chain Name
	ChainID        int               // Chain ID
	Endpoint       string            // JSONRPC Endpoint
	From           string            // Relayer Address
	KeystorePath   string            // Directory of keystore
	BlockstorePath string            // Directory of data
	FreshStart     bool              // Flag indicating that the relayer should start from genesis
	Opts           map[string]string // Other config
}

func RunBridge(chains []Chain) error {
	sysErr := make(chan error)
	c := core.NewCore(sysErr)

	for _, chain := range chains {
		chainConfig := &core.ChainConfig{
			Name:           chain.Name,
			Id:             msg.ChainId(chain.ChainID),
			Endpoint:       chain.Endpoint,
			From:           chain.From,
			KeystorePath:   chain.KeystorePath,
			BlockstorePath: chain.BlockstorePath,
			Opts:           chain.Opts,
			Insecure:       false,
			FreshStart:     true,
			LatestBlock:    false,
		}
		logger := log.Root().New("chain", chainConfig.Name)
		newChain, err := ethereum.InitializeChain(chainConfig, logger, sysErr, nil)
		if err != nil {
			return err
		}
		c.AddChain(newChain)
	}
	c.Start()
	return nil
}
