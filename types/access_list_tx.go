package types

// import (
// 	"fmt"
// )

type TxAccessList []AccessTuple

type AccessTuple struct {
	Address     Address
	StorageKeys []Hash
}

// type Tx
