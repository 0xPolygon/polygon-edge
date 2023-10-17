package e2e

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// ArgGroup is the type of the third argument of HRM contract
type ArgGroup struct {
	name   string
	number uint
	flag   bool
	people ArgHumans
}

// String returns text for predeploy flag
func (g *ArgGroup) String() string {
	return fmt.Sprintf(
		"[%s, %d, %t, %s]",
		g.name,
		g.number,
		g.flag,
		g.people.String(),
	)
}

// ArgGroups is a collection of ArgGroup
type ArgGroups []ArgGroup

// String returns the text for predeploy flag
func (gs *ArgGroups) String() string {
	groupStrs := make([]string, len(*gs))

	for i, group := range *gs {
		groupStrs[i] = group.String()
	}

	return fmt.Sprintf("[%s]", strings.Join(groupStrs, ","))
}

// ArgGroup is the type of the third argument of HRM contract
type ArgHuman struct {
	addr   string
	name   string
	number int
}

// String returns the text for predeploy flag
func (h *ArgHuman) String() string {
	return fmt.Sprintf("[%s, %s, %d]", h.addr, h.name, h.number)
}

// ArgHumans is a collection of ArgHuman
type ArgHumans []ArgHuman

// String returns the text for predeploy flag
func (hs *ArgHumans) String() string {
	humanStrs := make([]string, len(*hs))

	for i, human := range *hs {
		humanStrs[i] = human.String()
	}

	return fmt.Sprintf("[%s]", strings.Join(humanStrs, ","))
}

func TestGenesis_Predeployment(t *testing.T) {
	t.Parallel()

	var (
		artifactPath = "./metadata/predeploy_abi.json"

		_, senderAddr = tests.GenerateKeyAndAddr(t)
		contractAddr  = types.StringToAddress("1200")

		// predeploy arguments
		id     = 1000000
		name   = "TestContract"
		groups = ArgGroups{
			{
				name:   "group1",
				number: 1,
				flag:   true,
				people: ArgHumans{
					{
						addr:   types.StringToAddress("1").String(),
						name:   "A",
						number: 11,
					},
					{
						addr:   types.StringToAddress("2").String(),
						name:   "B",
						number: 12,
					},
				},
			},
			{
				name:   "group2",
				number: 2,
				flag:   false,
				people: ArgHumans{
					{
						addr:   types.StringToAddress("3").String(),
						name:   "C",
						number: 21,
					},
					{
						addr:   types.StringToAddress("4").String(),
						name:   "D",
						number: 22,
					},
				},
			},
		}
	)

	artifactsPath, err := filepath.Abs(artifactPath)
	if err != nil {
		t.Fatalf("unable to get working directory, %v", err)
	}

	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(_ int, config *framework.TestServerConfig) {
			config.Premine(senderAddr, framework.EthToWei(10))
			config.SetPredeployParams(&framework.PredeployParams{
				ArtifactsPath:    artifactsPath,
				PredeployAddress: contractAddr.String(),
				ConstructorArgs: []string{
					fmt.Sprintf("%d", id),
					name,
					groups.String(),
				},
			})
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	t.Cleanup(func() {
		cancel()
	})

	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	clt := srv.JSONRPC()

	// Extract the contract ABI from the metadata test file
	content, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("unable to open JSON file, %v", err)
	}

	predeployABI := abi.MustNewABI(
		extractABIFromJSONBody(string(content)),
	)

	callSCMethod := func(t *testing.T, methodName string, args ...interface{}) interface{} {
		t.Helper()

		method, ok := predeployABI.Methods[methodName]
		assert.Truef(t, ok, "%s method not present in SC", methodName)

		data := method.ID()

		if len(args) > 0 {
			input, err := method.Inputs.Encode(args)
			assert.NoError(t, err)

			data = append(data, input...)
		}

		toAddress := ethgo.Address(contractAddr)

		response, err := clt.Eth().Call(
			&ethgo.CallMsg{
				From:     ethgo.Address(senderAddr),
				To:       &toAddress,
				GasPrice: 100000000,
				Value:    big.NewInt(0),
				Data:     data,
			},
			ethgo.BlockNumber(1),
		)
		assert.NoError(t, err, "failed to call SC method")

		byteResponse, decodeError := hex.DecodeHex(response)
		if decodeError != nil {
			t.Fatalf("unable to decode hex response, %v", decodeError)
		}

		decodedResults, err := method.Outputs.Decode(byteResponse)
		if err != nil {
			t.Fatalf("unable to decode response, %v", err)
		}

		results, ok := decodedResults.(map[string]interface{})
		if !ok {
			t.Fatal("failed type assertion from decodedResults to map")
		}

		return results["0"]
	}

	t.Run("id is set correctly", func(t *testing.T) {
		t.Parallel()

		rawResID := callSCMethod(t, "getID")
		resID, ok := rawResID.(*big.Int)

		assert.Truef(t, ok, "failed to cast the result to *big.Int, actual %T", rawResID)
		assert.Zero(
			t,
			big.NewInt(int64(id)).Cmp(resID),
		)
	})

	t.Run("name is set correctly", func(t *testing.T) {
		t.Parallel()

		rawResName := callSCMethod(t, "getName")
		resName, ok := rawResName.(string)

		assert.Truef(t, ok, "failed to cast the result to string, actual %T", rawResName)
		assert.Equal(
			t,
			name,
			resName,
		)
	})

	testHuman := func(t *testing.T, groupIndex int, humanIndex int) {
		t.Helper()

		human := groups[groupIndex].people[humanIndex]

		t.Run("human addr is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResAddr := callSCMethod(t, "getHumanAddr", groupIndex, humanIndex)
			resAddr, ok := rawResAddr.(ethgo.Address)

			assert.Truef(t, ok, "failed to cast the result to ethgo.Address, actual %T", rawResAddr)
			assert.Equal(
				t,
				human.addr,
				resAddr.String(),
			)
		})

		t.Run("human name is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResName := callSCMethod(t, "getHumanName", groupIndex, humanIndex)
			resName, ok := rawResName.(string)

			assert.Truef(t, ok, "failed to cast the result to string, actual %T", rawResName)
			assert.Equal(
				t,
				human.name,
				resName,
			)
		})

		t.Run("human number is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResNumber := callSCMethod(t, "getHumanNumber", groupIndex, humanIndex)
			resNumber, ok := rawResNumber.(*big.Int)

			assert.Truef(t, ok, "failed to cast the result to *big.Int, actual %T", resNumber)
			assert.Zero(
				t,
				big.NewInt(int64(human.number)).Cmp(resNumber),
			)
		})
	}

	testGroup := func(t *testing.T, groupIndex int) {
		t.Helper()

		group := groups[groupIndex]

		t.Run("group name is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResName := callSCMethod(t, "getGroupName", groupIndex)
			resName, ok := rawResName.(string)

			assert.Truef(t, ok, "failed to cast the result to string, actual %T", rawResName)
			assert.Equal(
				t,
				group.name,
				resName,
			)
		})

		t.Run("group number is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResNumber := callSCMethod(t, "getGroupNumber", groupIndex)
			resNumber, ok := rawResNumber.(*big.Int)

			assert.Truef(t, ok, "failed to cast the result to int, actual %T", rawResNumber)
			assert.Zero(
				t,
				big.NewInt(int64(group.number)).Cmp(resNumber),
			)
		})

		t.Run("group flag is set correctly", func(t *testing.T) {
			t.Parallel()

			rawResFlag := callSCMethod(t, "getGroupFlag", groupIndex)
			resFlag, ok := rawResFlag.(bool)

			assert.Truef(t, ok, "failed to cast the result to bool, actual %T", rawResFlag)
			assert.Equal(
				t,
				group.flag,
				resFlag,
			)
		})

		for humanIndex := range group.people {
			humanIndex := humanIndex
			t.Run(fmt.Sprintf("groups[%d].people[%d] is set correctly", groupIndex, humanIndex), func(t *testing.T) {
				t.Parallel()

				testHuman(t, groupIndex, humanIndex)
			})
		}
	}

	for idx := range groups {
		idx := idx
		t.Run(fmt.Sprintf("groups[%d] is set correctly", idx), func(t *testing.T) {
			t.Parallel()

			testGroup(t, idx)
		})
	}
}
