package precompiled

import (
	_ "embed"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

//go:embed console.sol
var consoleContract string

var logOverloads = map[string]*abi.Type{}

func init() {
	rxp := regexp.MustCompile("abi.encodeWithSignature\\(\"log(.*)\"")
	matches := rxp.FindAllStringSubmatch(consoleContract, -1)

	for _, match := range matches {
		signature := match[1]

		// parse the type of the console call. Note that 'uint'
		// objects are defined without bytes (i.e. 256).
		typ, err := abi.NewType("tuple" + signature)
		if err != nil {
			log.Fatal(fmt.Errorf("BUG: Failed to parse %s", signature))
		}

		// signature of the call. Use the version without the bytes in 'uint'.
		sig := ethgo.Keccak256([]byte("log" + match[1]))[:4]
		logOverloads[hex.EncodeToString(sig)] = typ
	}
}

func decodeConsole(input []byte) (val []string) {
	sig := hex.EncodeToString(input[:4])
	logSig, ok := logOverloads[sig]

	if !ok {
		return
	}

	input = input[4:]
	raw, err := logSig.Decode(input)

	if err != nil {
		return
	}

	valuesMap, ok := raw.(map[string]interface{})
	if !ok {
		return
	}

	val = []string{}
	for _, v := range valuesMap {
		val = append(val, fmt.Sprint(v))
	}

	return
}

// console is a debug precompile contract that simulates the `console.sol` functionality
type console struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract
func (c *console) gas(_ []byte, _ *chain.ForksInTime) uint64 {
	return 0
}

// Run contains the implementation logic of the precompiled contract
func (c *console) run(input []byte, _ types.Address, _ runtime.Host) ([]byte, error) {
	fmt.Printf("Console: %v\n", decodeConsole(input))

	return nil, nil
}
