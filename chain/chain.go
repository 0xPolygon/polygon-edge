package chain

type Chain struct {
	Genesis   *Genesis
	Params    *Params
	Bootnodes Bootnodes
}

type Bootnodes []string
