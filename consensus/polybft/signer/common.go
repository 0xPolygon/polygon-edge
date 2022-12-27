package bls

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

const MapToModeSolidity = true

var (
	domain, _ = hex.DecodeString("508e30424791cb9a71683381558c3da1979b6fa423b2d6db1396b1d94d7c4a78")

	maxBigInt, _ = new(big.Int).SetString("30644e72e131a029b85045b68181585d2833e84879b9709143e1f593f0000001", 16)

	ellipticCurveG2 = &G2{
		X: Fp2{
			[2]Fp{
				newFp(0x8e83b5d102bc2026, 0xdceb1935497b0172, 0xfbb8264797811adf, 0x19573841af96503b),
				newFp(0xafb4737da84c6140, 0x6043dd5a5802d8c4, 0x09e950fc52a02f86, 0x14fef0833aea7b6b),
			},
		},
		Y: Fp2{
			[2]Fp{
				newFp(0x619dfa9d886be9f6, 0xfe7fd297f59e9b78, 0xff9e1a62231b7dfe, 0x28fd7eebae9e4206),
				newFp(0x64095b56c71856ee, 0xdc57f922327d3cbb, 0x55f935be33351076, 0x0da4a0e693fd6482),
			},
		},
		Z: Fp2{
			[2]Fp{
				newFp(0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f),
				newFp(0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000),
			},
		},
	}

	r1 = newFp(0xd35d438dc58f0d9d, 0x0a78eb28f5c70b3d, 0x666ea36f7879462c, 0x0e0a77c19a07df2f)

	r2 = newFp(0xf32cfc5b538afa89, 0xb5e71911d44501fb, 0x47ab1eff0a417ff6, 0x06d89f71cab8351f)

	qCoef []uint64
)

func init() {
	if err := InitCurve(CurveSNARK1); err != nil {
		panic(fmt.Errorf("snark1 curve initialization error: %w", err))
	}

	if err := SetMapToMode(0); err != nil {
		panic(fmt.Errorf("snark1 curve map to mode: %w", err))
	}

	qCoef = PrecomputeG2(ellipticCurveG2)
}

// Returns bls/bn254 domain
func GetDomain() []byte {
	return domain
}

func GetCoef() []uint64 {
	return qCoef
}

func G1Zero(g1 *G1) *G1 {
	g1.X = newFp(0, 0, 0, 0)
	g1.Z = newFp(0, 0, 0, 0)

	FpAdd(&g1.Y, &g1.Y, &r1)

	return g1
}

func G2Zero(g2 *G2) *G2 {
	g2.X.D[0] = newFp(0, 0, 0, 0)
	g2.X.D[1] = newFp(0, 0, 0, 0)
	g2.Y.D[1] = newFp(0, 0, 0, 0)
	g2.Z.D[0] = newFp(0, 0, 0, 0)
	g2.Z.D[1] = newFp(0, 0, 0, 0)

	FpAdd(&g2.Y.D[0], &g2.Y.D[0], &r1)

	return g2
}
