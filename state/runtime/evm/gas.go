package evm

// Sha3 gas prices
const (
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

const (
	// CallValueTransferGas paid for CALL when the value transfer is non-zero.
	CallValueTransferGas uint64 = 9000
	// CallNewAccountGas paid for CALL when the destination address didn't exist prior.
	CallNewAccountGas uint64 = 25000
	// CallStipend is the free gas given at beginning of call.
	CallStipend uint64 = 2300
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
	// SuicideRefundGas refunded following a suicide operation.
	SuicideRefundGas uint64 = 24000
	// CopyGas multiplied by the number of words copied
	CopyGas uint64 = 3
)
