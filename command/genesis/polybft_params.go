package genesis

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"path"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"

	rootchain "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	premineValidatorsFlag          = "premine-validators"
	polyBftValidatorPrefixPathFlag = "validator-prefix"
	smartContractsRootPathFlag     = "contracts-path"

	validatorSetSizeFlag = "validator-set-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	validatorsFlag       = "polybft-validators"
	bridgeFlag           = "bridge"

	defaultEpochSize                  = uint64(10)
	defaultSprintSize                 = uint64(5)
	defaultValidatorSetSize           = 100
	defaultBlockTime                  = 2 * time.Second
	defaultPolyBftValidatorPrefixPath = "test-chain-"
	defaultBridge                     = false

	bootnodePortStart = 30301

	WeiScalingFactor = 1_000_000_000_000_000_000 // 10^18
)

var (
	GenesisContracts = []struct {
		Name         string
		RelativePath string
		Address      types.Address
	}{
		{
			// Validator contract
			Name:         "ChildValidatorSet",
			RelativePath: "child/ChildValidatorSet.sol",
			Address:      contracts.ValidatorSetContract,
		},
		{
			// State receiver contract
			Name:         "StateReceiver",
			RelativePath: "child/StateReceiver.sol",
			Address:      contracts.StateReceiverContract,
		},
		{
			// Native Token contract (Matic ERC-20)
			Name:         "MRC20",
			RelativePath: "child/MRC20.sol",
			Address:      contracts.NativeTokenContract,
		},
		{
			// BLS contract
			Name:         "BLS",
			RelativePath: "common/BLS.sol",
			Address:      contracts.BLSContract,
		},
		{
			// Merkle contract
			Name:         "Merkle",
			RelativePath: "common/Merkle.sol",
			Address:      contracts.MerkleContract,
		},
	}
)

func (p *genesisParams) GeneratePolyBFTConfig(validatorsInfo []GenesisTarget, governanceAddress types.Address, stake map[types.Address]*big.Int) (*chain.Chain, error) {
	allocs, err := p.deployContracts()
	if err != nil {
		return nil, err
	}

	// use 1st account as governance address
	polyBftConfig := &polybft.PolyBFTConfig{
		BlockTime:         p.blockTime,
		EpochSize:         p.epochSize,
		SprintSize:        p.sprintSize,
		ValidatorSetSize:  p.validatorSetSize,
		ValidatorSetAddr:  contracts.ValidatorSetContract,
		StateReceiverAddr: contracts.StateReceiverContract,
		Governance:        governanceAddress,
	}

	if p.bridgeEnabled {
		ip, err := rootchain.ReadRootchainIP()
		if err != nil {
			return nil, err
		}

		polyBftConfig.Bridge = &polybft.BridgeConfig{
			BridgeAddr:      rootchain.StateSenderAddress,
			CheckpointAddr:  rootchain.CheckpointManagerAddress,
			JSONRPCEndpoint: ip,
		}
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			ChainID: int(p.chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	// set generic validators as bootnodes if needed
	if len(p.bootnodes) == 0 {
		for i, validator := range validatorsInfo {
			bnode := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bnode)
		}
	}

	var premine []string = nil
	if len(p.premine) > 0 {
		premine = p.premine
	} else if p.premineValidators != "" {
		premine = make([]string, len(validatorsInfo))
		for i, vi := range validatorsInfo {
			premine[i] = fmt.Sprintf("%s:%s",
				vi.Account.Ecdsa.Address().String(), p.premineValidators)
		}
	}

	//setup stake for validator set
	//todo stake shouldn be a part of prealloc
	fmt.Println("alloc", allocs)
	for k, v := range stake {
		genAcc := allocs[k]
		if genAcc == nil {
			genAcc = &chain.GenesisAccount{}
		}
		genAcc.Balance = v
		allocs[k] = genAcc
	}

	// premine accounts
	if err := fillPremineMap(allocs, premine); err != nil {
		return nil, err
	}

	// set initial validator set
	genesisValidators, err := p.getGenesisValidators(validatorsInfo, allocs)
	if err != nil {
		return nil, err
	}

	polyBftConfig.InitialValidatorSet = genesisValidators

	pubKeys := make([]*bls.PublicKey, len(validatorsInfo))
	for i, validatorInfo := range validatorsInfo {
		pubKeys[i] = validatorInfo.Account.Bls.PublicKey()
	}

	genesisExtraData, err := generateExtraDataPolyBft(genesisValidators, pubKeys)
	if err != nil {
		return nil, err
	}

	// populate genesis parameters
	chainConfig.Genesis = &chain.Genesis{
		GasLimit:   p.blockGasLimit,
		Difficulty: 0,
		Alloc:      allocs,
		ExtraData:  genesisExtraData,
		GasUsed:    command.DefaultGenesisGasUsed,
		Mixhash:    polybft.PolyBFTMixDigest,
	}

	return chainConfig, nil
}

func (p *genesisParams) getGenesisValidators(validators []GenesisTarget,
	allocs map[types.Address]*chain.GenesisAccount) ([]*polybft.Validator, error) {
	result := make([]*polybft.Validator, 0)

	if len(p.validators) > 0 {
		for _, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) < 2 {
				continue
			}

			addr := types.StringToAddress(parts[0])

			balance, err := getBalanceInWei(addr, allocs)
			if err != nil {
				return nil, err
			}

			result = append(result, &polybft.Validator{
				Address: addr,
				BlsKey:  parts[1],
				Balance: balance,
			})
		}
	} else {
		for _, validator := range validators {
			pubKeyMarshalled := validator.Account.Bls.PublicKey().Marshal()
			addr := types.Address(validator.Account.Ecdsa.Address())

			balance, err := getBalanceInWei(addr, allocs)
			if err != nil {
				return nil, err
			}

			result = append(result, &polybft.Validator{
				Address: addr,
				BlsKey:  hex.EncodeToString(pubKeyMarshalled),
				Balance: balance,
			})
		}
	}

	return result, nil
}

// getBalanceInWei returns balance for genesis account based on its address.
// If not found in provided allocations map, 1M native tokens is returned.
func getBalanceInWei(address types.Address, allocations map[types.Address]*chain.GenesisAccount) (*big.Int, error) {
	if genesisAcc, ok := allocations[address]; ok {
		return genesisAcc.Balance, nil
	}

	val := command.DefaultPremineBalance

	amount, err := types.ParseUint256orHex(&val)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount %s: %w", val, err)
	}

	return amount, nil
}

func (p *genesisParams) generatePolyBftGenesis() error {
	validatorsInfo, err := ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.polyBftValidatorPrefixPath)
	if err != nil {
		return err
	}

	config, err := params.GeneratePolyBFTConfig(validatorsInfo, types.Address(validatorsInfo[0].Account.Ecdsa.Address()), nil)
	if err != nil {
		return err
	}

	return helper.WriteGenesisConfigToDisk(config, params.genesisPath)
}

func (p *genesisParams) deployContracts() (map[types.Address]*chain.GenesisAccount, error) {

	allocations := make(map[types.Address]*chain.GenesisAccount, len(GenesisContracts))

	fmt.Println("smartContractsRootPath", p.smartContractsRootPath)
	for _, c := range GenesisContracts {
		artifact, err := polybft.ReadArtifact(p.smartContractsRootPath, c.RelativePath, c.Name)
		if err != nil {
			return nil, err
		}

		allocations[c.Address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    artifact.DeployedBytecode,
		}
	}

	return allocations, nil
}

// generateExtraDataPolyBft populates Extra with specific fields required for polybft consensus protocol
func generateExtraDataPolyBft(validators []*polybft.Validator, publicKeys []*bls.PublicKey) ([]byte, error) {
	if len(validators) != len(publicKeys) {
		return nil, fmt.Errorf("expected same length for genesis validators and BLS public keys")
	}

	delta := &polybft.ValidatorSetDelta{
		Added:   make(polybft.AccountSet, len(validators)),
		Removed: bitmap.Bitmap{},
	}

	for i, validator := range validators {
		delta.Added[i] = &polybft.ValidatorMetadata{
			Address:     validator.Address,
			BlsKey:      publicKeys[i],
			VotingPower: convertWeiToTokensAmount(validator.Balance).Uint64(),
		}
	}

	extra := polybft.Extra{Validators: delta}

	return append(make([]byte, polybft.ExtraVanity), extra.MarshalRLPTo(nil)...), nil
}

// convertWeiToTokensAmount converts provided wei balance to tokens amount
func convertWeiToTokensAmount(weiBalance *big.Int) *big.Int {
	return weiBalance.Div(weiBalance, big.NewInt(WeiScalingFactor))
}
