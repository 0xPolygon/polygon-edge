package ibft

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
)

type Config struct {
	RequestTimeout uint64         `toml:",omitempty"` // The timeout for each Istanbul round in milliseconds.
	BlockPeriod    uint64         `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy ProposerPolicy `toml:",omitempty"` // The policy for proposer selection
	Epoch          uint64         `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
}

var DefaultConfig = &Config{
	RequestTimeout: 10000,
	BlockPeriod:    1,
	ProposerPolicy: RoundRobin,
	Epoch:          30000,
}
