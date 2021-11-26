package loadbot

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

type Configuration struct {
	TPS           uint64
	AccountsCount uint64
	Value         int64
	Count         uint64
	JSONRPCs      []string
	GRPCs         []string
}

type Metrics struct {
	Duration                   time.Duration
	TotalTransactionsSentCount uint64
	FailedTransactionsCount    uint64
}

// generateRandomValue creates a random value used in a transaction.
// The max value that can be generated represents 0.01 ETH.
func generateRandomValue() (*big.Int, error) {
	b, err := rand.Int(rand.Reader, big.NewInt(10000000000000000))
	if err != nil {
		return nil, fmt.Errorf("failed to create random number: %v", err)
	}
	return b, nil
}

func Run(conf *Configuration) (error, *Metrics) {
	return nil, nil
}
