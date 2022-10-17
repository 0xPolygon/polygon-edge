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
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/secrets"
	secretsHelper "github.com/0xPolygon/polygon-edge/secrets/helper"
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

	defaultEpochSize                  = uint64(10)
	defaultSprintSize                 = uint64(5)
	defaultValidatorSetSize           = 100
	defaultBlockTime                  = 2 * time.Second
	defaultPolyBftValidatorPrefixPath = "test-chain-"
	defaultContractsRootFolder        = "v3-contracts/artifacts/contracts"

	bootnodePortStart = 30301
)

func (p *genesisParams) generatePolyBFTConfig() (*chain.Chain, error) {
	validatorsInfo, err := ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.polyBftValidatorPrefixPath)
	if err != nil {
		return nil, err
	}

	alloc, err := p.deployContracts()
	if err != nil {
		return nil, err
	}

	// use test-chain-1 as governance address (if found). If the validators from
	// utils.ReadValidatorsByRegexp are read in order, we could use that list instead.
	// The important point is to have `test-chain-1` as the governance address for the
	// e2e register command.
	secretsManager, err := secretsHelper.SetupLocalSecretsManager("./test-chain-1")
	if err != nil {
		return nil, err
	}

	governanceAccount, err := wallet.GenerateNewAccountFromSecret(secretsManager, secrets.ValidatorBLSKey)
	if err != nil {
		return nil, err
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		InitialValidatorSet: p.getGenesisValidators(validatorsInfo),
		BlockTime:           p.blockTime,
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		ValidatorSetSize:    p.validatorSetSize,
		ValidatorSetAddr:    contracts.ValidatorSetContract,
		StateReceiverAddr:   contracts.StateReceiverContract,
		Governance:          types.Address(governanceAccount.Ecdsa.Address()),
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 0,
			Alloc:      alloc,
			ExtraData:  generateExtraDataPolyBft(validatorsInfo),
			GasUsed:    command.DefaultGenesisGasUsed,
			Mixhash:    polybft.PolyMixDigest,
		},
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

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, premine); err != nil {
		return nil, err
	}

	return chainConfig, nil
}

func (p *genesisParams) getGenesisValidators(validators []GenesisTarget) (result []*polybft.Validator) {
	if len(p.validators) > 0 {
		for _, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) < 2 {
				continue
			}

			result = append(result, &polybft.Validator{
				Address: types.StringToAddress(parts[0]),
				BlsKey:  parts[1],
				Balance: big.NewInt(1000), // hard-coded balance
			})
		}
	} else {
		for _, validator := range validators {
			pubKeyMarshalled := validator.Account.Bls.PublicKey().Marshal()

			result = append(result, &polybft.Validator{
				Address: types.Address(validator.Account.Ecdsa.Address()),
				BlsKey:  hex.EncodeToString(pubKeyMarshalled),
				Balance: big.NewInt(1000), // hard-coded balance
			})
		}
	}

	return result
}

func (p *genesisParams) generatePolyBftGenesis() error {
	config, err := params.generatePolyBFTConfig()
	if err != nil {
		return err
	}

	return helper.WriteGenesisConfigToDisk(config, params.genesisPath)
}

func (p *genesisParams) deployContracts() (map[types.Address]*chain.GenesisAccount, error) {
	genesisContracts := []struct {
		name         string
		relativePath string
		address      types.Address
	}{
		{
			// Validator contract
			name:         "ChildValidatorSet",
			relativePath: "child/ChildValidatorSet.sol",
			address:      contracts.ValidatorSetContract,
		},
		{
			// State receiver contract
			name:         "StateReceiver",
			relativePath: "child/StateReceiver.sol",
			address:      contracts.StateReceiverContract,
		},
		{
			// Native Token contract (Matic ERC-20)
			name:         "MRC20",
			relativePath: "child/MRC20.sol",
			address:      contracts.NativeTokenContract,
		},
		{
			// BLS contract
			name:         "BLS",
			relativePath: "common/BLS.sol",
			address:      contracts.BLSContract,
		},
		{
			// Merkle contract
			name:         "Merkle",
			relativePath: "common/Merkle.sol",
			address:      contracts.MerkleContract,
		},
	}

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts))

	for _, contract := range genesisContracts {
		artifact, err := polybftcontracts.ReadArtifact(p.smartContractsRootPath, contract.relativePath, contract.name)
		if err != nil {
			return nil, err
		}

		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    artifact.DeployedBytecode,
		}
	}

	return allocations, nil
}

func generateExtraDataPolyBft(validators []GenesisTarget) []byte {
	delta := &polybft.ValidatorSetDelta{
		Added:   make(polybft.AccountSet, len(validators)),
		Removed: bitmap.Bitmap{},
	}

	for i, validator := range validators {
		delta.Added[i] = &polybft.ValidatorAccount{
			Address: types.Address(validator.Account.Ecdsa.Address()),
			BlsKey:  validator.Account.Bls.PublicKey(),
		}
	}

	extra := polybft.Extra{Validators: delta}

	return append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...)
}
