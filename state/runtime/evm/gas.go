package evm

/*
// Precompiled contract gas prices
const (
	// EcrecoverGas is the elliptic curve sender recovery gas price
	EcrecoverGas uint64 = 3000
	// Sha256BaseGas is the base price for a SHA256 operation
	Sha256BaseGas uint64 = 60
	// Sha256PerWordGas is the per-word price for a SHA256 operation
	Sha256PerWordGas uint64 = 12
	// Ripemd160BaseGas is the base price for a RIPEMD160 operation
	Ripemd160BaseGas uint64 = 600
	// Ripemd160PerWordGas is the per-word price for a RIPEMD160 operation
	Ripemd160PerWordGas uint64 = 120
	// IdentityBaseGas is the base price for a data copy operation
	IdentityBaseGas uint64 = 15
	// IdentityPerWordGas is the per-work price for a data copy operation
	IdentityPerWordGas uint64 = 3
	// ModExpQuadCoeffDiv is the divisor for the quadratic particle of the big int modular exponentiation
	ModExpQuadCoeffDiv uint64 = 20
	// Bn256AddGas is the gas needed for an elliptic curve addition
	Bn256AddGas uint64 = 500
	// Bn256ScalarMulGas is the gas needed for an elliptic curve scalar multiplication
	Bn256ScalarMulGas uint64 = 40000
	// Bn256PairingBaseGas is the base price for an elliptic curve pairing check
	Bn256PairingBaseGas uint64 = 100000
	// Bn256PairingPerPointGas is the per-point price for an elliptic curve pairing check
	Bn256PairingPerPointGas uint64 = 80000
)
*/

// Sha3 gas prices
const (
	// Sha3Gas once per SHA3 operation.
	Sha3Gas uint64 = 30
	// Sha3WordGas once per word of the SHA3 operation's data
	Sha3WordGas uint64 = 6
)

// SStore gas prices
const (
	// SstoreSetGas once per SLOAD operation.
	SstoreSetGas uint64 = 20000
	// SstoreResetGas once per SSTORE operation if the zeroness changes from zero.
	SstoreResetGas uint64 = 5000
	// SstoreClearGas once per SSTORE operation if the zeroness doesn't change.
	SstoreClearGas uint64 = 5000
	// SstoreRefundGas once per SSTORE operation if the zeroness changes to zero.
	SstoreRefundGas uint64 = 15000

	// NetSstoreNoopGas once per SSTORE operation if the value doesn't change.
	NetSstoreNoopGas uint64 = 200
	// NetSstoreInitGas once per SSTORE operation from clean zero.
	NetSstoreInitGas uint64 = 20000
	// NetSstoreCleanGas once per SSTORE operation from clean non-zero.
	NetSstoreCleanGas uint64 = 5000
	// NetSstoreDirtyGas once per SSTORE operation from dirty.
	NetSstoreDirtyGas uint64 = 200

	// NetSstoreClearRefund once per SSTORE operation for clearing an originally existing storage slot
	NetSstoreClearRefund uint64 = 15000
	// NetSstoreResetRefund once per SSTORE operation for resetting to the original non-zero value
	NetSstoreResetRefund uint64 = 4800
	// NetSstoreResetClearRefund once per SSTORE operation for resetting to the original zero value
	NetSstoreResetClearRefund uint64 = 19800
)

// Fixed gas costs
const (
	GasQuickStep    uint64 = 2
	GasFastestStep  uint64 = 3
	GasFastStep     uint64 = 5
	GasMidStep      uint64 = 8
	GasSlowStep     uint64 = 10
	GasExtStep      uint64 = 20
	GasReturn       uint64 = 0
	GasStop         uint64 = 0
	GasContractByte uint64 = 200
	MemoryGas       uint64 = 3
	QuadCoeffDiv    uint64 = 512
)

const (
	// CallValueTransferGas paid for CALL when the value transfer is non-zero.
	CallValueTransferGas uint64 = 9000
	// CallNewAccountGas paid for CALL when the destination address didn't exist prior.
	CallNewAccountGas uint64 = 25000
	// CallStipend is the free gas given at beginning of call.
	CallStipend uint64 = 2300
	// CallCreateDepth is the maximum depth of call/create stack.
	CallCreateDepth uint64 = 1024
)

const (
	// LogGas per LOG* operation.
	LogGas uint64 = 375
	// LogTopicGas multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.
	LogTopicGas uint64 = 375
	// LogDataGas is the per byte in a LOG* operation's data.
	LogDataGas uint64 = 8
)

const (
	// CreateGas once per CREATE operation & contract-creation transaction.
	CreateGas uint64 = 32000
	// Create2Gas once per CREATE2 operation
	Create2Gas uint64 = 32000
	// CreateDataGas paid for the return data in a contract-creation
	CreateDataGas uint64 = 200
)

const (
	// MaxCodeSize maximum bytecode to permit for a contract
	MaxCodeSize = 24576
	// SuicideRefundGas refunded following a suicide operation.
	SuicideRefundGas uint64 = 24000
	// CopyGas multiplied by the number of words copied
	CopyGas uint64 = 3
)
