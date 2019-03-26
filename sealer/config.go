package sealer

import "time"

const (
	minCommitInterval = 1 * time.Second
)

// Config is the sealer config
type Config struct {
	CommitInterval time.Duration
	// TODO, Instant sealer
}

// DefaultConfig is the default sealer config
func DefaultConfig() *Config {
	return &Config{
		CommitInterval: 10 * time.Second,
	}
}
