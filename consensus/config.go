package consensus

import "math/big"

// Fork is a fork in the chain
type Fork struct {
	block *big.Int
}

// Active checks if the fork is active
func (f *Fork) Active(n *big.Int) bool {
	if f.block == nil {
		return false
	}
	return f.block.Cmp(n) <= 0
}

// ChainConfig is the config related to the chain
type ChainConfig struct {
	HomesteadBlock      Fork
	ByzantiumBlock      Fork
	ConstantinopleBlock Fork
}

func NewChainConfig(h, b, c uint64) *ChainConfig {
	cc := &ChainConfig{
		HomesteadBlock:      Fork{},
		ByzantiumBlock:      Fork{},
		ConstantinopleBlock: Fork{},
	}
	if h != 0 {
		cc.HomesteadBlock = Fork{block: big.NewInt(int64(h))}
	}
	if b != 0 {
		cc.ByzantiumBlock = Fork{block: big.NewInt(int64(b))}
	}
	if c != 0 {
		cc.ConstantinopleBlock = Fork{block: big.NewInt(int64(c))}
	}
	return cc
}
