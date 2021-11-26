package loadbot

import "time"

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

func Run(conf *Configuration) (error, *Metrics) {
	return nil, nil
}
