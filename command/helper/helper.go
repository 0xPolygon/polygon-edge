package helper

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	helperFlags "github.com/0xPolygon/polygon-sdk/helper/flags"
	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/helper/staking"
	"github.com/0xPolygon/polygon-sdk/server"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/mitchellh/cli"
	"github.com/ryanuber/columnize"
	"google.golang.org/grpc"
)

const (
	GenesisFileName       = "./genesis.json"
	DefaultChainName      = "example"
	DefaultChainID        = 100
	DefaultPremineBalance = "0x3635C9ADC5DEA00000" // 1000 ETH
	DefaultConsensus      = "pow"
	DefaultPriceLimit     = 1
	DefaultMaxSlots       = 4096
	GenesisGasUsed        = 458752               // 0x70000
	GenesisGasLimit       = 5242880              // 0x500000
	DefaultStakedBalance  = "0x8AC7230489E80000" // 10 ETH
	StakingSCBytecode     = "0x6080604052600436106100555760003560e01c80632def66201461005a578063373d6132146100715780633a4b66f11461009c57806350d68ed8146100a6578063ca1e7819146100d1578063f90ecacc146100fc575b600080fd5b34801561006657600080fd5b5061006f610139565b005b34801561007d57600080fd5b50610086610351565b6040516100939190610b36565b60405180910390f35b6100a461035b565b005b3480156100b257600080fd5b506100bb6105d6565b6040516100c89190610b1b565b60405180910390f35b3480156100dd57600080fd5b506100e66105e2565b6040516100f39190610ab9565b60405180910390f35b34801561010857600080fd5b50610123600480360381019061011e9190610979565b610670565b6040516101309190610a9e565b60405180910390f35b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054116101bb576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016101b290610adb565b60405180910390fd5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205490506000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff16156102a05761029f336106af565b5b80600460008282546102b29190610bf1565b925050819055503373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f193505050501580156102ff573d6000803e3d6000fd5b503373ffffffffffffffffffffffffffffffffffffffff167f0f5bb82176feb1b5e747e28471aa92156a04d9f3ab9f45f28e2d704232b93f75826040516103469190610b36565b60405180910390a250565b6000600454905090565b346004600082825461036d9190610b9b565b9250508190555034600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546103c39190610b9b565b92505081905550600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff1615801561047d5750670de0b6b3a76400006fffffffffffffffffffffffffffffffff16600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410155b156105865760018060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff021916908315150217905550600080549050600360003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506000339080600181540180825580915050600190039060005260206000200160009091909190916101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055505b3373ffffffffffffffffffffffffffffffffffffffff167f9e71bc8eea02a63969f509818f2dafb9254532904319f9dbda79b67bd34a5f3d346040516105cc9190610b36565b60405180910390a2565b670de0b6b3a764000081565b6060600080548060200260200160405190810160405280929190818152602001828054801561066657602002820191906000526020600020905b8160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001906001019080831161061c575b5050505050905090565b6000818154811061068057600080fd5b906000526020600020016000915054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b600080549050600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410610735576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161072c90610afb565b60405180910390fd5b6000600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205490506000600160008054905061078d9190610bf1565b905080821461087b5760008082815481106107ab576107aa610cdb565b5b9060005260206000200160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905080600084815481106107ed576107ec610cdb565b5b9060005260206000200160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555082600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550505b6000600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055506000600360008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550600080548061092a57610929610cac565b5b6001900381819060005260206000200160006101000a81549073ffffffffffffffffffffffffffffffffffffffff02191690559055505050565b60008135905061097381610d61565b92915050565b60006020828403121561098f5761098e610d0a565b5b600061099d84828501610964565b91505092915050565b60006109b283836109be565b60208301905092915050565b6109c781610c25565b82525050565b6109d681610c25565b82525050565b60006109e782610b61565b6109f18185610b79565b93506109fc83610b51565b8060005b83811015610a2d578151610a1488826109a6565b9750610a1f83610b6c565b925050600181019050610a00565b5085935050505092915050565b6000610a47601d83610b8a565b9150610a5282610d0f565b602082019050919050565b6000610a6a601283610b8a565b9150610a7582610d38565b602082019050919050565b610a8981610c37565b82525050565b610a9881610c73565b82525050565b6000602082019050610ab360008301846109cd565b92915050565b60006020820190508181036000830152610ad381846109dc565b905092915050565b60006020820190508181036000830152610af481610a3a565b9050919050565b60006020820190508181036000830152610b1481610a5d565b9050919050565b6000602082019050610b306000830184610a80565b92915050565b6000602082019050610b4b6000830184610a8f565b92915050565b6000819050602082019050919050565b600081519050919050565b6000602082019050919050565b600082825260208201905092915050565b600082825260208201905092915050565b6000610ba682610c73565b9150610bb183610c73565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff03821115610be657610be5610c7d565b5b828201905092915050565b6000610bfc82610c73565b9150610c0783610c73565b925082821015610c1a57610c19610c7d565b5b828203905092915050565b6000610c3082610c53565b9050919050565b60006fffffffffffffffffffffffffffffffff82169050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b600080fd5b7f4f6e6c79207374616b65722063616e2063616c6c2066756e6374696f6e000000600082015250565b7f696e646578206f7574206f662072616e67650000000000000000000000000000600082015250565b610d6a81610c73565b8114610d7557600080fd5b5056fea26469706673582212200732d127c069747c9fba40cde21178279bf118074d859232ff634cd4c0c2678964736f6c63430008070033"
)

// FlagDescriptor contains the description elements for a command flag
type FlagDescriptor struct {
	Description       string   // Flag description
	Arguments         []string // Arguments list
	ArgumentsOptional bool     // Flag indicating if flag arguments are optional
	FlagOptional      bool
}

// GetDescription gets the flag description
func (fd *FlagDescriptor) GetDescription() string {
	return fd.Description
}

// GetArgumentsList gets the list of arguments for the flag
func (fd *FlagDescriptor) GetArgumentsList() []string {
	return fd.Arguments
}

// AreArgumentsOptional checks if the flag arguments are optional
func (fd *FlagDescriptor) AreArgumentsOptional() bool {
	return fd.ArgumentsOptional
}

// IsFlagOptional checks if the flag itself is optional
func (fd *FlagDescriptor) IsFlagOptional() bool {
	return fd.FlagOptional
}

// GenerateHelp is a utility function called by every command's Help() method
func GenerateHelp(synopsys string, usage string, flagMap map[string]FlagDescriptor) string {
	helpOutput := ""

	flagCounter := 0
	for flagEl, descriptor := range flagMap {
		helpOutput += GenerateFlagDesc(flagEl, descriptor) + "\n"
		flagCounter++

		if flagCounter < len(flagMap) {
			helpOutput += "\n"
		}
	}

	if len(flagMap) > 0 {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n\nFlags:\n\n%s", synopsys, usage, helpOutput)
	} else {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n", synopsys, usage)
	}
}

// GenerateFlagDesc generates the flag descriptions in a readable format
func GenerateFlagDesc(flagEl string, descriptor FlagDescriptor) string {
	// Generate the top row (with various flags)
	topRow := fmt.Sprintf("--%s", flagEl)

	argumentsOptional := descriptor.AreArgumentsOptional()
	argumentsList := descriptor.GetArgumentsList()

	argLength := len(argumentsList)

	if argLength > 0 {
		topRow += " "
		if argumentsOptional {
			topRow += "["
		}

		for argIndx, argument := range argumentsList {
			topRow += argument

			if argIndx < argLength-1 && argLength > 1 {
				topRow += " "
			}
		}

		if argumentsOptional {
			topRow += "]"
		}
	}

	// Generate the bottom description
	bottomRow := fmt.Sprintf("\t%s", descriptor.GetDescription())

	return fmt.Sprintf("%s\n%s", topRow, bottomRow)
}

// GenerateUsage is a helper function for generating command usage text
func GenerateUsage(baseCommand string, flagMap map[string]FlagDescriptor) string {
	output := baseCommand + " "

	maxFlagsPerLine := 3 // Just an arbitrary value, can be anything reasonable

	var addedFlags int // Keeps track of when a newline character needs to be inserted
	for flagEl, descriptor := range flagMap {
		// Open the flag bracket
		if descriptor.IsFlagOptional() {
			output += "["
		}

		// Add the actual flag name
		output += fmt.Sprintf("--%s", flagEl)

		// Open the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += " ["
		}

		argumentsList := descriptor.GetArgumentsList()

		// Add the flag arguments list
		for argIndex, argument := range argumentsList {
			if argIndex == 0 {
				// Only called for the first argument
				output += " "
			}

			output += argument

			if argIndex < len(argumentsList)-1 {
				output += " "
			}
		}

		// Close the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += "]"
		}

		// Close the flag bracket
		if descriptor.IsFlagOptional() {
			output += "]"
		}

		addedFlags++
		if addedFlags%maxFlagsPerLine == 0 {
			output += "\n\t"
		} else {
			output += " "
		}
	}

	return output
}

// HandleSignals is a helper method for handling signals sent to the console
// Like stop, error, etc.
func HandleSignals(closeFn func(), ui cli.Ui) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-signalCh

	output := fmt.Sprintf("\n[SIGNAL] Caught signal: %v\n", sig)
	output += "Gracefully shutting down client...\n"

	ui.Output(output)

	// Call the Minimal server close callback
	gracefulCh := make(chan struct{})
	go func() {
		if closeFn != nil {
			closeFn()
		}
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return 1
	case <-time.After(5 * time.Second):
		return 1
	case <-gracefulCh:
		return 0
	}
}

const (
	StatError   = "StatError"
	ExistsError = "ExistsError"
)

// GenesisGenError is a specific error type for generating genesis
type GenesisGenError struct {
	message   string
	errorType string
}

// GetMessage returns the message of the genesis generation error
func (g *GenesisGenError) GetMessage() string {
	return g.message
}

// GetType returns the type of the genesis generation error
func (g *GenesisGenError) GetType() string {
	return g.errorType
}

// VerifyGenesisExistence checks if the genesis file at the specified path is present
func VerifyGenesisExistence(genesisPath string) *GenesisGenError {
	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("failed to stat (%s): %v", genesisPath, err),
			errorType: StatError,
		}
	}
	if !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("genesis file at path (%s) already exists", genesisPath),
			errorType: ExistsError,
		}
	}

	return nil
}

// FillPremineMap fills the premine map for the genesis.json file with passed in balances and accounts
func FillPremineMap(
	premineMap map[types.Address]*chain.GenesisAccount,
	premine helperFlags.ArrayFlags,
) error {
	for _, prem := range premine {
		var addr types.Address
		val := DefaultPremineBalance
		if indx := strings.Index(prem, ":"); indx != -1 {
			// <addr>:<balance>
			addr, val = types.StringToAddress(prem[:indx]), prem[indx+1:]
		} else {
			// <addr>
			addr = types.StringToAddress(prem)
		}

		amount, err := types.ParseUint256orHex(&val)
		if err != nil {
			return fmt.Errorf("failed to parse amount %s: %v", val, err)
		}
		premineMap[addr] = &chain.GenesisAccount{
			Balance: amount,
		}
	}

	return nil
}

// WriteGenesisToDisk writes the passed in configuration to a genesis.json file at the specified path
func WriteGenesisToDisk(chain *chain.Chain, genesisPath string) error {
	data, err := json.MarshalIndent(chain, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to generate genesis: %w", err)
	}
	if err := ioutil.WriteFile(genesisPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write genesis: %w", err)
	}

	return nil
}

type devGenesisParams struct {
	chainName string
	premine   helperFlags.ArrayFlags
	gasLimit  uint64
	chainID   uint64
}

// generateDevGenesis generates a base dev genesis file with premined balances
func generateDevGenesis(params devGenesisParams) error {
	genesisPath := filepath.Join(".", GenesisFileName)

	generateError := VerifyGenesisExistence(genesisPath)

	if generateError != nil {
		switch generateError.GetType() {
		case StatError:
			// Unable to stat file
			return errors.New(generateError.GetMessage())
		case ExistsError:
			// Not an error for the dev command, it shouldn't regenerate the genesis
			return nil
		}
	}

	cc := &chain.Chain{
		Name: params.chainName,
		Genesis: &chain.Genesis{
			GasLimit:   params.gasLimit,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  []byte{},
			GasUsed:    GenesisGasUsed,
		},
		Params: &chain.Params{
			ChainID: int(params.chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				"dev": map[string]interface{}{},
			},
		},
		Bootnodes: []string{},
	}

	if err := PredeployStakingSC(
		cc.Genesis.Alloc,
		[]types.Address{},
	); err != nil {
		return err
	}

	if err := FillPremineMap(cc.Genesis.Alloc, params.premine); err != nil {
		return err
	}

	return WriteGenesisToDisk(cc, genesisPath)
}

// BootstrapDevCommand creates a config and generates the dev genesis file
func BootstrapDevCommand(baseCommand string, args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{
		Network: &Network{
			NoDiscover: true,
			MaxPeers:   0,
		},
		TxPool: &TxPool{},
	}
	cliConfig.Seal = true
	cliConfig.Dev = true
	cliConfig.Chain = "genesis.json"

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var premine helperFlags.ArrayFlags
	var gaslimit uint64
	var chainID uint64

	flags.StringVar(&cliConfig.LogLevel, "log-level", DefaultConfig().LogLevel, "")
	flags.Var(&premine, "premine", "")
	flags.StringVar(&cliConfig.TxPool.Locals, "locals", "", "")
	flags.BoolVar(&cliConfig.TxPool.NoLocals, "nolocals", false, "")
	flags.Uint64Var(&cliConfig.TxPool.PriceLimit, "price-limit", DefaultPriceLimit, "")
	flags.Uint64Var(&cliConfig.TxPool.MaxSlots, "max-slots", DefaultMaxSlots, "")
	flags.Uint64Var(&gaslimit, "block-gas-limit", GenesisGasLimit, "")
	flags.Uint64Var(&cliConfig.DevInterval, "dev-interval", 0, "")
	flags.Uint64Var(&chainID, "chainid", DefaultChainID, "")
	flags.StringVar(&cliConfig.BlockGasTarget, "block-gas-target", strconv.FormatUint(0, 10), "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if err := config.mergeConfigWith(cliConfig); err != nil {
		return nil, err
	}

	if err := generateDevGenesis(devGenesisParams{
		chainName: config.Chain,
		premine:   premine,
		gasLimit:  gaslimit,
		chainID:   chainID,
	}); err != nil {
		return nil, err
	}

	return config, nil
}

func ReadConfig(baseCommand string, args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{
		Network: &Network{},
		TxPool:  &TxPool{},
	}

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var configFile string
	flags.StringVar(&cliConfig.LogLevel, "log-level", "", "")
	flags.BoolVar(&cliConfig.Seal, "seal", false, "")
	flags.StringVar(&configFile, "config", "", "")
	flags.StringVar(&cliConfig.Chain, "chain", "", "")
	flags.StringVar(&cliConfig.DataDir, "data-dir", "", "")
	flags.StringVar(&cliConfig.GRPCAddr, "grpc", "", "")
	flags.StringVar(&cliConfig.JSONRPCAddr, "jsonrpc", "", "")
	flags.StringVar(&cliConfig.Join, "join", "", "")
	flags.StringVar(&cliConfig.Network.Addr, "libp2p", "", "")
	flags.StringVar(&cliConfig.Network.NatAddr, "nat", "", "the external IP address without port, as can be seen by peers")
	flags.StringVar(&cliConfig.Network.Dns, "dns", "", " the host DNS address which can be used by a remote peer for connection")
	flags.BoolVar(&cliConfig.Network.NoDiscover, "no-discover", false, "")
	flags.Uint64Var(&cliConfig.Network.MaxPeers, "max-peers", 0, "")
	flags.StringVar(&cliConfig.TxPool.Locals, "locals", "", "")
	flags.BoolVar(&cliConfig.TxPool.NoLocals, "nolocals", false, "")
	flags.Uint64Var(&cliConfig.TxPool.PriceLimit, "price-limit", DefaultPriceLimit, "")
	flags.Uint64Var(&cliConfig.TxPool.MaxSlots, "max-slots", DefaultMaxSlots, "")
	flags.BoolVar(&cliConfig.Dev, "dev", false, "")
	flags.Uint64Var(&cliConfig.DevInterval, "dev-interval", 0, "")
	flags.StringVar(&cliConfig.BlockGasTarget, "block-gas-target", strconv.FormatUint(0, 10), "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if configFile != "" {
		// A config file has been passed in, parse it
		diskConfigFile, err := readConfigFile(configFile)
		if err != nil {
			return nil, err
		}

		if err := config.mergeConfigWith(diskConfigFile); err != nil {
			return nil, err
		}
	}

	if err := config.mergeConfigWith(cliConfig); err != nil {
		return nil, err
	}

	return config, nil
}

type HelpGenerator interface {
	DefineFlags()
}

// Meta is a helper utility for the commands
type Meta struct {
	UI   cli.Ui
	Addr string

	FlagMap map[string]FlagDescriptor
}

// DefineFlags sets global flags used by several commands
func (m *Meta) DefineFlags() {
	m.FlagMap = make(map[string]FlagDescriptor)

	m.FlagMap["grpc-address"] = FlagDescriptor{
		Description: fmt.Sprintf("Address of the gRPC API. Default: %s:%d", "127.0.0.1", server.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// FlagSet adds some default commands to handle grpc connections with the server
func (m *Meta) FlagSet(n string) *flag.FlagSet {
	f := flag.NewFlagSet(n, flag.ContinueOnError)
	f.StringVar(&m.Addr, "grpc-address", fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort), "")

	return f
}

// Conn returns a grpc connection
func (m *Meta) Conn() (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(m.Addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	return conn, nil
}

// OUTPUT FORMATTING //

// FormatList formats a list, using a specific blank value replacement
func FormatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"

	return columnize.Format(in, columnConf)
}

// FormatKV formats key value pairs:
//
// Key = Value
//
// Key = <none>
func FormatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "

	return columnize.Format(in, columnConf)
}

// DirectoryExists checks if the directory at the specified path exists
func DirectoryExists(directoryPath string) bool {
	// Grab the absolute filepath
	pathAbs, err := filepath.Abs(directoryPath)
	if err != nil {
		return false
	}

	// Check if the directory exists, and that it's actually a directory if there is a hit
	if fileInfo, statErr := os.Stat(pathAbs); os.IsNotExist(statErr) || (fileInfo != nil && !fileInfo.IsDir()) {
		return false
	}

	return true
}

// PredeployStakingSC is a helper method for setting up the staking smart contract account,
// using the passed in validators as prestaked validators
func PredeployStakingSC(
	premineMap map[types.Address]*chain.GenesisAccount,
	validators []types.Address,
) error {
	// Set the code for the staking smart contract
	// Code retrieved from https://github.com/0xPolygon/staking-contracts
	scHex, _ := hex.DecodeHex(StakingSCBytecode)
	stakingAccount := &chain.GenesisAccount{
		Code: scHex,
	}

	// Parse the default staked balance value into *big.Int
	val := DefaultStakedBalance
	bigDefaultStakedBalance, err := types.ParseUint256orHex(&val)
	if err != nil {
		return fmt.Errorf("unable to generate DefaultStatkedBalance, %v", err)
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)
	bigTrueValue := big.NewInt(1)
	stakedAmount := big.NewInt(0)
	for indx, validator := range validators {
		// Update the total staked amount
		stakedAmount.Add(stakedAmount, bigDefaultStakedBalance)

		// Get the storage indexes
		storageIndexes := staking.GetStorageIndexes(validator, int64(indx))

		// Set the value for the validators array
		storageMap[types.BytesToHash(storageIndexes.ValidatorsIndex)] = types.BytesToHash(
			validator.Bytes(),
		)

		// Set the value for the address -> validator array index mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToIsValidatorIndex)] = types.BytesToHash(bigTrueValue.Bytes())

		// Set the value for the address -> staked amount mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToStakedAmountIndex)] = types.StringToHash(hex.EncodeBig(bigDefaultStakedBalance))

		// Set the value for the address -> validator index mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToValidatorIndexIndex)] = types.StringToHash(hex.EncodeUint64(uint64(indx)))

		// Set the value for the total staked amount
		storageMap[types.BytesToHash(storageIndexes.StakedAmountIndex)] = types.BytesToHash(stakedAmount.Bytes())

		// Set the value for the size of the validators array
		storageMap[types.BytesToHash(storageIndexes.ValidatorsArraySizeIndex)] = types.StringToHash(hex.EncodeUint64(uint64(indx + 1)))
	}

	// Save the storage map
	stakingAccount.Storage = storageMap

	// Set the Staking SC balance to numValidators * defaultStakedBalance
	stakingAccount.Balance = stakedAmount

	// Add the account to the premine map so the executor can apply it to state
	premineMap[types.StringToAddress("1001")] = stakingAccount

	return nil
}
