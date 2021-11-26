package loadbot

type Configuration struct {
	TPS           uint64
	AccountsCount uint64
	Value         int64
	Count         uint64
	JSONRPCs      []string
	GRPCs         []string
}


