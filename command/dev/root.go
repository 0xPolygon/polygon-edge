package dev

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/command"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/sealer"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"
	"github.com/0xPolygon/minimal/types"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Dev starts a testing blockchain ",
	RunE:  versionRunE,
}

func init() {
	command.RegisterCmd(devCmd)
}

func versionRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, versionRunE)
}

func versionRunE(cmd *cobra.Command, args []string) error {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "",
		Level: hclog.LevelFromString("INFO"),
	})

	// Create an instant consensus
	engine := &consensus.NoProof{}

	// inmemory blockchain storage
	inmemBlockchainStorage, _ := memory.NewMemoryStorage(logger)

	// inmemory state storage
	inmemStateStorage := itrie.NewMemoryStorage()

	// create the state
	stateI := itrie.NewState(inmemStateStorage)

	// everything enabled
	params := &chain.Params{
		Forks: &chain.Forks{
			Homestead:      chain.NewFork(0),
			Byzantium:      chain.NewFork(0),
			Constantinople: chain.NewFork(0),
			Petersburg:     chain.NewFork(0),
			EIP150:         chain.NewFork(0),
			EIP155:         chain.NewFork(0),
			EIP158:         chain.NewFork(0),
		},
		ChainID: 1234,
	}

	// blockchain object
	genesis := &chain.Genesis{
		Difficulty: 10,
		Timestamp:  10,
		Alloc: chain.GenesisAlloc{
			types.StringToAddress("0x1100000000000000000000000000000000000005"): chain.GenesisAccount{
				Balance: big.NewInt(985162418487296000),
			},
		},
	}

	executor := state.NewExecutor(params, stateI)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	bChain := blockchain.NewBlockchain(inmemBlockchainStorage, params, engine, executor)
	if err := bChain.WriteGenesis(genesis); err != nil {
		panic(err)
	}

	executor.GetHash = bChain.GetHashHelper

	// sealer
	config := &sealer.Config{
		DevMode:  true,
		Coinbase: types.StringToAddress("111111"),
	}
	sealer := sealer.NewSealer(config, logger, bChain, engine, executor)

	// create the jsonrpc command
	m := &minimal.Minimal{}
	m.Blockchain = bChain
	m.Sealer = sealer

	// start the sealer once all the process are active
	sealer.SetEnabled(true)

	// wait
	handleClose(closeFn)
	return nil
}

func handleClose(closeFn func()) error {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	fmt.Printf("Caught signal: %v\n", sig)
	fmt.Printf("Gracefully shutting down agent...\n")

	gracefulCh := make(chan struct{}, 1)
	go func() {
		closeFn()
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return errors.New("Forced exit")
	case <-time.After(5 * time.Second):
		return errors.New("Timed out when exiting the agent")
	case <-gracefulCh:
	}
	return nil
}
