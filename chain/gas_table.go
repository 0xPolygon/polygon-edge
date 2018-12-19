package chain

const (
	// TxGas per transaction not creating a contract. NOTE: Not payable on data of calls between transactions.
	TxGas uint64 = 21000
	// TxGasContractCreation per transaction that creates a contract. NOTE: Not payable on data of calls between transactions.
	TxGasContractCreation uint64 = 53000
	// TxDataZeroGas per byte of data attached to a transaction that equals zero. NOTE: Not payable on data of calls between transactions.
	TxDataZeroGas uint64 = 4
	// TxDataNonZeroGas per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
	TxDataNonZeroGas uint64 = 68
)

// GasTable stores the gas cost for the variable opcodes
type GasTable struct {
	ExtcodeSize     uint64
	ExtcodeCopy     uint64
	ExtcodeHash     uint64
	Balance         uint64
	SLoad           uint64
	Calls           uint64
	Suicide         uint64
	ExpByte         uint64
	CreateBySuicide uint64
}

// GasTableHomestead contain the gas prices for the homestead phase.
var GasTableHomestead = GasTable{
	ExtcodeSize: 20,
	ExtcodeCopy: 20,
	Balance:     20,
	SLoad:       50,
	Calls:       40,
	Suicide:     0,
	ExpByte:     10,
}

// GasTableEIP150 contain the gas prices for the EIP150 phase.
var GasTableEIP150 = GasTable{
	ExtcodeSize:     700,
	ExtcodeCopy:     700,
	Balance:         400,
	SLoad:           200,
	Calls:           700,
	Suicide:         5000,
	ExpByte:         10,
	CreateBySuicide: 25000,
}

// GasTableEIP158 contain the gas prices for the EIP158 phase.
var GasTableEIP158 = GasTable{
	ExtcodeSize:     700,
	ExtcodeCopy:     700,
	Balance:         400,
	SLoad:           200,
	Calls:           700,
	Suicide:         5000,
	ExpByte:         50,
	CreateBySuicide: 25000,
}

// GasTableConstantinople contain the gas prices for the constantinople phase.
var GasTableConstantinople = GasTable{
	ExtcodeSize:     700,
	ExtcodeCopy:     700,
	ExtcodeHash:     400,
	Balance:         400,
	SLoad:           200,
	Calls:           700,
	Suicide:         5000,
	ExpByte:         50,
	CreateBySuicide: 25000,
}
