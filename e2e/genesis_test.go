package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/assert"
)

// TestGenesisBlockGasLimit tests the genesis block limit setting
func TestGenesisBlockGasLimit(t *testing.T) {
	testTable := []struct {
		name                  string
		blockGasLimit         uint64
		expectedBlockGasLimit uint64
	}{
		{
			"Custom block gas limit",
			5000000000,
			5000000000,
		},
		{
			"Default block gas limit",
			0,
			command.DefaultGenesisGasLimit,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			_, addr := tests.GenerateKeyAndAddr(t)

			ibftManager := framework.NewIBFTServersManager(
				t,
				1,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(addr, framework.EthToWei(10))
					config.SetBlockTime(1)

					if testCase.blockGasLimit != 0 {
						config.SetBlockLimit(testCase.blockGasLimit)
					}
				},
			)

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			ibftManager.StartServers(ctx)
			srv := ibftManager.GetServer(0)

			client := srv.JSONRPC()

			block, err := client.Eth().GetBlockByNumber(0, true)
			if err != nil {
				t.Fatalf("failed to retrieve block: %v", err)
			}

			assert.Equal(t, testCase.expectedBlockGasLimit, block.GasLimit)
		})
	}
}

func extractABIFromJSONBody(body string) string {
	r, _ := regexp.Compile("\"abi\": \\[(?s).*\\]")

	match := r.FindString(body)

	return match[strings.Index(match, "["):]
}

func TestGenesis_Predeployment(t *testing.T) {
	var (
		firstParam  = "a"
		secondParam = "b"
		numValue    = "123"
	)

	_, senderAddr := tests.GenerateKeyAndAddr(t)
	addrStr := "0x01110"
	predeployAddress := types.StringToAddress(addrStr)

	artifactsPath, err := filepath.Abs("./metadata/predeploySC.json")
	if err != nil {
		t.Fatalf("unable to get working directory, %v", err)
	}

	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.Premine(senderAddr, framework.EthToWei(10))
			config.SetPredeployParams(&framework.PredeployParams{
				ArtifactsPath:    artifactsPath,
				PredeployAddress: addrStr,
				ConstructorArgs: []string{
					fmt.Sprintf("[[%s],[%s]]", firstParam, secondParam), numValue, firstParam},
			})
		})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	clt := srv.JSONRPC()

	// Extract the contract ABI from the metadata test file
	content, err := ioutil.ReadFile("./metadata/predeploySC.json")
	if err != nil {
		t.Fatalf("unable to open JSON file, %v", err)
	}

	predeployABI := abi.MustNewABI(
		extractABIFromJSONBody(string(content)),
	)

	greetMethod, ok := predeployABI.Methods["greet"]
	if !ok {
		t.Fatalf("greet method not present in SC")
	}

	greetNumMethod, ok := predeployABI.Methods["greetNum"]
	if !ok {
		t.Fatalf("greetNum method not present in SC")
	}

	greetSecondMethod, ok := predeployABI.Methods["greetSecond"]
	if !ok {
		t.Fatalf("greet method not present in SC")
	}

	contractMethods := []*abi.Method{greetMethod, greetNumMethod, greetSecondMethod}

	toAddress := ethgo.Address(predeployAddress)

	executeSCCall := func(methodABI *abi.Method) interface{} {
		response, err := clt.Eth().Call(
			&ethgo.CallMsg{
				From:     ethgo.Address(senderAddr),
				To:       &toAddress,
				Data:     methodABI.ID(),
				GasPrice: 100000000,
				Value:    big.NewInt(0),
			},
			ethgo.BlockNumber(1),
		)

		if err != nil {
			t.Fatalf("unable to execute SC call, %v", err)
		}

		byteResponse, decodeError := hex.DecodeHex(response)
		if decodeError != nil {
			t.Fatalf("unable to decode hex response, %v", decodeError)
		}

		decodedResults, err := methodABI.Outputs.Decode(byteResponse)
		if err != nil {
			t.Fatalf("unable to decode response, %v", err)
		}

		results, ok := decodedResults.(map[string]interface{})
		if !ok {
			t.Fatal("failed type assertion from decodedResults to map")
		}

		return results["0"]
	}

	var (
		resultsLock sync.Mutex
		wg          sync.WaitGroup
	)

	callResults := make([]interface{}, 3)

	addCallResults := func(index int, result interface{}) {
		resultsLock.Lock()
		defer resultsLock.Unlock()

		callResults[index] = result
	}

	for i, contractMethod := range contractMethods {
		wg.Add(1)

		go func(i int, method *abi.Method) {
			defer wg.Done()

			addCallResults(i, executeSCCall(method))
		}(i, contractMethod)
	}

	wg.Wait()

	greeting, ok := callResults[0].(string)
	if !ok {
		t.Fatal("failed type assertion to string")
	}

	assert.Equal(t, secondParam, greeting)

	num, ok := callResults[1].(*big.Int)
	if !ok {
		t.Fatal("failed type assertion to big.Int")
	}

	convNumValue, _ := strconv.ParseUint(numValue, 10, 64)
	assert.Equal(t, convNumValue, num.Uint64())

	secondGreeting, ok := callResults[2].(string)
	if !ok {
		t.Fatal("failed type assertion to string")
	}

	assert.Equal(t, firstParam, secondGreeting)
}
