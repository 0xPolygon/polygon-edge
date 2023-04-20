package framework

import "math/big"

func EthToWei(ethValue int64) *big.Int {
	return EthToWeiPrecise(ethValue, 18)
}

func EthToWeiPrecise(ethValue int64, decimals int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil))
}
