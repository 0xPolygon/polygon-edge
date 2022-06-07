package jsonrpc

import "github.com/umbracle/ethgo"

type Debug struct {
	c *Client
}

// Eth returns the reference to the eth namespace
func (c *Client) Debug() *Debug {
	return c.endpoints.d
}

type TransactionTrace struct {
	Gas         uint64
	ReturnValue string
	StructLogs  []*StructLogs
}

type StructLogs struct {
	Depth   int
	Gas     int
	GasCost int
	Op      string
	Pc      int
	Memory  []string
	Stack   []string
	Storage map[string]string
}

func (d *Debug) TraceTransaction(hash ethgo.Hash) (*TransactionTrace, error) {
	var res *TransactionTrace
	err := d.c.Call("debug_traceTransaction", &res, hash)
	return res, err
}
