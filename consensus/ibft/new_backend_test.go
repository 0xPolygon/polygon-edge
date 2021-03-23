package ibft

import (
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft/aux"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/hashicorp/go-hclog"
)

func TestBackend(t *testing.T) {
	store := aux.NewMockBlockchain()

	bb := &Backend2{
		logger:  hclog.New(&hclog.LoggerOptions{Level: hclog.Debug}),
		config:  DefaultConfig, // TODO: Decode from consensusConfig
		store:   store,
		address: crypto.PubKeyToAddress(&privateKey.PublicKey),
	}

	if err := bb.Setup(); err != nil {
		return nil, err
	}
	go bb.run()

	// time.Sleep(5 * time.Second)
}
