package chain

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

var GasTableHomestead = &GasTable{
	ExtcodeSize: 20,
	ExtcodeCopy: 20,
	Balance:     20,
	SLoad:       50,
	Calls:       40,
	Suicide:     0,
	ExpByte:     10,
}

var GasTableEIP150 = &GasTable{
	ExtcodeSize:     700,
	ExtcodeCopy:     700,
	Balance:         400,
	SLoad:           200,
	Calls:           700,
	Suicide:         5000,
	ExpByte:         10,
	CreateBySuicide: 25000,
}

var GasTableEIP158 = &GasTable{
	ExtcodeSize:     700,
	ExtcodeCopy:     700,
	Balance:         400,
	SLoad:           200,
	Calls:           700,
	Suicide:         5000,
	ExpByte:         50,
	CreateBySuicide: 25000,
}

var GasTableConstantinople = &GasTable{
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
