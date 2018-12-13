package chain

// Params are all the set of params for the chain
type Params struct {
	Forks *Forks `json:"number"`
}

// Forks specifies when each fork is activated
type Forks struct {
	Homestead      Fork `json:"homestead"`
	Byzantium      Fork `json:"byzantium"`
	Constantinople Fork `json:"constantinople"`
	EIP150         Fork `json:"EIP150"`
	EIP158         Fork `json:"EIP158"`
}

func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
		Homestead:      f.Homestead.Active(block),
		Byzantium:      f.Byzantium.Active(block),
		Constantinople: f.Constantinople.Active(block),
		EIP150:         f.EIP150.Active(block),
		EIP158:         f.EIP158.Active(block),
	}
}

type Fork uint64

func (f Fork) Active(block uint64) bool {
	return block >= uint64(f)
}

type ForksInTime struct {
	Homestead, Byzantium, Constantinople, EIP150, EIP158 bool
}
