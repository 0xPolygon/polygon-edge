package tests

import "math/big"

func EthToWei(ethValue int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}
