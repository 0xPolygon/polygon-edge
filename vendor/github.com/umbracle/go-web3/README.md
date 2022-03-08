
# Go-Web3

## JsonRPC

```golang
package main

import (
	"fmt"
	
	web3 "github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

func main() {
	client, err := jsonrpc.NewClient("https://mainnet.infura.io")
	if err != nil {
		panic(err)
	}

	number, err := client.Eth().BlockNumber()
	if err != nil {
		panic(err)
	}

	header, err := client.Eth().GetBlockByNumber(web3.BlockNumber(number), true)
	if err != nil {
		panic(err)
	}

	fmt.Println(header)
}
```

## ABI

The ABI codifier uses randomized tests with e2e integration tests with a real Geth client to ensure that the codification is correct and provides the same results as the AbiEncoder from Solidity. 

To use the library import:

```
"github.com/umbracle/go-web3/abi"
```

Declare basic objects:

```golang
typ, err := abi.NewType("uint256")
```

or 

```golang
typ = abi.MustNewType("uint256")
```

and use it to encode/decode the data:

```golang
num := big.NewInt(1)

encoded, err := typ.Encode(num)
if err != nil {
    panic(err)
}

decoded, err := typ.Decode(encoded) // decoded as interface
if err != nil {
    panic(err)
}

num2 := decoded.(*big.Int)
fmt.Println(num.Cmp(num2) == 0) // num == num2
```

You can also codify structs as Solidity tuples:

```golang
import (
	"fmt"
    
	web3 "github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	"math/big"
)

func main() {
	typ := abi.MustNewType("tuple(address a, uint256 b)")

	type Obj struct {
		A web3.Address
		B *big.Int
	}
	obj := &Obj{
		A: web3.Address{0x1},
		B: big.NewInt(1),
	}

	// Encode
	encoded, err := typ.Encode(obj)
	if err != nil {
		panic(err)
	}

	// Decode output into a map
	res, err := typ.Decode(encoded)
	if err != nil {
		panic(err)
	}

	// Decode into a struct
	var obj2 Obj
	if err := typ.DecodeStruct(encoded, &obj2); err != nil {
		panic(err)
	}

	fmt.Println(res)
	fmt.Println(obj)
}
```

## Wallet

As for now the library only provides primitive abstractions to send signed abstractions. The intended goal is to abstract the next steps inside the contract package.

```golang
// Generate a random wallet
key, _ := wallet.GenerateKey()

to := web3.Address{0x1}
transferVal := big.NewInt(1000)

// Create the transaction
txn := &web3.Transaction{
	To:    &to,
	Value: transferVal,
	Gas:   100000,
}

// Create the signer object and sign
signer := wallet.NewEIP155Signer(chainID)
txn, _ = signer.SignTx(txn, key)

// Send the signed transaction
data := txn.MarshalRLP()
hash, _ := c.Eth().SendRawTransaction(data)
```

## ENS

Resolve names on the Ethereum Name Service registrar.

```golang
import (
    "fmt"

    "github.com/umbracle/go-web3/jsonrpc"
    "github.com/umbracle/go-web3/ens"
)

func main() {
	client, err := jsonrpc.NewClient("https://mainnet.infura.io")
    if err != nil {
        panic(err)
    }

	ens, err := ens.NewENS(ens.WithClient(client))
	if err != nil {
		panic(err)
	}
	addr, err := ens.Resolve("ens_address")
	if err != nil {
		panic(err)
	}
    fmt.Println(addr)
}
```

## Block tracker

```golang
import (
    "fmt"

    web3 "github.com/umbracle/go-web3"
    "github.com/umbracle/go-web3/jsonrpc"
    "github.com/umbracle/go-web3/blocktracker"
)

func main() {
	client, err := jsonrpc.NewClient("https://mainnet.infura.io")
    if err != nil {
        panic(err)
    }

	tracker = blocktracker.NewBlockTracker(client, WithBlockMaxBacklog(1000))
	if err := tracker.Init(); err != nil {
		panic(err)
	}
	go tracker.Start()

	sub := tracker.Subscribe()
	go func() {
		for {
			select {
			case evnt := <-sub:
				fmt.Println(evnt)
			case <-ctx.Done():
				return
			}
		}
	}
}
```

## Tracker

Complete example of the tracker [here](./tracker/README.md)
