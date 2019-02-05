package blockchain

import "fmt"

type GasPool struct {
	gas uint64
}

func NewGasPool(gas uint64) *GasPool {
	return &GasPool{gas}
}

func (g *GasPool) SubGas(amount uint64) error {
	if g.gas < amount {
		return fmt.Errorf("gas limit reached")
	}
	g.gas -= amount
	return nil
}

func (g *GasPool) AddGas(amount uint64) {
	g.gas += amount
}
