package chain

// Params are all the set of params for the chain
type Params struct {
	Forks *Forks `json:"number"`
}

// Forks specifies when each fork is activated
type Forks struct {
	Homestead      uint64 `json:"homestead"`
	Byzantium      uint64 `json:"byzantium"`
	Constantinople uint64 `json:"constantinople"`
	EIP150         uint64 `json:"EIP150"`
	EIP158         uint64 `json:"EIP158"`
}

func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
		Homestead:      block >= f.Homestead,
		Byzantium:      block >= f.Byzantium,
		Constantinople: block >= f.Constantinople,
		EIP150:         block >= f.EIP150,
		EIP158:         block >= f.EIP158,
	}
}

type ForksInTime struct {
	Homestead, Byzantium, Constantinople, EIP150, EIP158 bool
}
