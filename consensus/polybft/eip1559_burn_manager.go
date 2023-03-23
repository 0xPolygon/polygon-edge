package polybft

import (
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/hashicorp/go-hclog"
)

var _ EIP1559BurnManager = (*eip1559BurnManager)(nil)

type EIP1559BurnManager interface {
	PostBlock(req *PostBlockRequest) error
}

// eip1559BurnConfig holds the configuration data of eip1559 burn manager
type eip1559BurnConfig struct {
	eip1559BurnAddr types.Address
	jsonrpcAddr     string
}

type eip1559BurnManager struct {
	logger  hclog.Logger
	config  *eip1559BurnConfig
	closeCh chan struct{}
}

func newEIP1559BurnManager(
	logger hclog.Logger,
	config *eip1559BurnConfig,
) *eip1559BurnManager {
	return &eip1559BurnManager{
		logger:  logger,
		config:  config,
		closeCh: make(chan struct{}),
	}
}

func (m *eip1559BurnManager) PostBlock(req *PostBlockRequest) error {

	panic("not implemented yet")
}
