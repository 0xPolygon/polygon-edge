
# Ethereum State

```
package main

import (
    "github.com/umbracle/minimal/state"
    "github.com/ethereum/go-ethereum/common"
)

func main() {

    addr1 := common.HexToAddress("1")   // 0x00...01
    addr2 := common.HexToAddress("2")   // 0x00...02

    state := state.NewState()
    txn := state.Txn()
    
    txn.SetBalance(addr1, big.NewInt(100))
    txn.SetBalance(addr2, big.NewInt(200))
    
    s := txn.Snapshot()
    txn.SetBalance(addr2, big.NewInt(300))
    txn.RevertToSnapshot(s)

    _, root := txn.Commit()
}
```
