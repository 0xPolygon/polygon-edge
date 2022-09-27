package e2e

import (
	"context"
	"crypto/ecdsa"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

// Test scenario:
// PoA [0, 29]
// - Check validator set in each validator equals to genesis validator
// - 3 validators stake to the validator contract after contract deploys at block #10
// PoS [30, ]
// - Check validator set in each validator has only 3 validators
func TestPoAPoSSwitch(t *testing.T) {
	var (
		// switch configuration
		posDeployContractAt = uint64(10)
		posStartAt          = uint64(30)

		defaultBalance = framework.EthToWei(1000)
		stakeAmount    = framework.EthToWei(5)
	)

	ibftManager := framework.NewIBFTServersManager(
		t,
		IBFTMinNodes,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.PremineValidatorBalance(defaultBalance)
		})

	// Set switch configuration into genesis.json
	err := ibftManager.GetServer(0).SwitchIBFTType(fork.PoS, posStartAt, nil, &posDeployContractAt)
	assert.NoError(t, err)

	// Get server slice
	servers := make([]*framework.TestServer, 0)
	for i := 0; i < IBFTMinNodes; i++ {
		servers = append(servers, ibftManager.GetServer(i))
	}

	// Get genesis validators
	genesisValidatorKeys := make([]*ecdsa.PrivateKey, IBFTMinNodes)
	genesisValidatorAddrs := make([]types.Address, IBFTMinNodes)

	for idx := 0; idx < IBFTMinNodes; idx++ {
		validatorKey, err := ibftManager.GetServer(idx).Config.PrivateKey()
		assert.NoError(t, err)

		validatorAddr := crypto.PubKeyToAddress(&validatorKey.PublicKey)
		genesisValidatorKeys[idx] = validatorKey
		genesisValidatorAddrs[idx] = validatorAddr
	}

	// Start servers
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	// Test in PoA
	validateSnapshot := func(
		ctx context.Context,
		srv *framework.TestServer,
		height uint64,
		expectedValidators []types.Address,
	) {
		res, err := srv.IBFTOperator().GetSnapshot(ctx, &ibftOp.SnapshotReq{
			Number: height,
		})
		assert.NoError(t, err)

		snapshotValidators := make([]types.Address, len(res.Validators))
		for idx, v := range res.Validators {
			snapshotValidators[idx] = types.BytesToAddress(v.Data)
		}

		assert.ElementsMatch(t, expectedValidators, snapshotValidators)
	}

	// Check validator set in each validator
	var wg sync.WaitGroup
	for i := 0; i < IBFTMinNodes; i++ {
		wg.Add(1)

		srv := ibftManager.GetServer(i)

		go func(srv *framework.TestServer) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
			defer cancel()

			// every validator should have 4 validators in validator set
			validateSnapshot(ctx, srv, 1, genesisValidatorAddrs)
		}(srv)
	}
	wg.Wait()

	// Wait until the staking contract is deployed
	waitErrors := framework.WaitForServersToSeal(servers, posDeployContractAt)
	if len(waitErrors) != 0 {
		t.Fatalf("Unable to wait for all nodes to seal blocks, %v", waitErrors)
	}

	// Stake balance
	// 3 genesis validators will stake but 1 gensis validator won't
	numStakedValidators := 3
	wg = sync.WaitGroup{}

	for idx := 0; idx < numStakedValidators; idx++ {
		wg.Add(1)

		srv := ibftManager.GetServer(idx)
		key, addr := genesisValidatorKeys[idx], genesisValidatorAddrs[idx]

		go func(srv *framework.TestServer, key *ecdsa.PrivateKey, addr types.Address) {
			defer wg.Done()

			err := framework.StakeAmount(
				addr,
				key,
				stakeAmount,
				srv,
			)
			assert.NoError(t, err)
		}(srv, key, addr)
	}
	wg.Wait()

	// Wait until PoS begins
	waitErrors = framework.WaitForServersToSeal(servers, posStartAt)
	if len(waitErrors) != 0 {
		t.Fatalf("Unable to wait for all nodes to seal blocks, %v", waitErrors)
	}

	expectedPoSValidators := genesisValidatorAddrs[:3]

	// Test in PoS
	wg = sync.WaitGroup{}

	for i := 0; i < IBFTMinNodes; i++ {
		wg.Add(1)

		srv := ibftManager.GetServer(i)

		go func(srv *framework.TestServer) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
			defer cancel()

			// every validator should have only 3 validators in validator set
			validateSnapshot(ctx, srv, posStartAt, expectedPoSValidators)
		}(srv)
	}
	wg.Wait()
}
