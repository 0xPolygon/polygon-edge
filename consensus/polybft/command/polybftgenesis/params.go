package polybftgenesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

const (
	dirFlag                 = "dir"
	nameFlag                = "name"
	premineFlag             = "premine"
	chainIDFlag             = "chain-id"
	blockGasLimitFlag       = "block-gas-limit"
	validatorPrefixPathFlag = "prefix"

	validatorSetSizeFlag = "validator-set-size"
	epochSizeFlag        = "epoch-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	validatorsFlag       = "polybft-validators"

	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second

	bootnodePortStart = 30301
)

var (
	// hard code address for the sidechain (this is only being used for testing)
	ValidatorSetAddr         = ethgo.HexToAddress("0xBd770416a3345F91E4B34576cb804a576fa48EB1")
	SidechainBridgeAddr      = ethgo.HexToAddress("0x5a443704dd4B594B382c22a083e2BD3090A6feF3")
	sidechainERC20Addr       = ethgo.HexToAddress("0x47e9Fbef8C83A1714F1951F142132E6e90F5fa5D")
	SidechainERC20BridgeAddr = ethgo.HexToAddress("0x8Be503bcdEd90ED42Eff31f56199399B2b0154CA")
)

var (
	errValidatorsNotSpecified = errors.New("validator information not specified")
	errUnsupportedConsensus   = errors.New("specified consensusRaw not supported")
	errInvalidEpochSize       = errors.New("epoch size must be greater than 1")
)

type genesisParams struct {
	genesisPath         string
	name                string
	validatorPrefixPath string
	premine             []string
	bootnodes           []string

	chainID       int
	blockGasLimit uint64

	validatorSetSize int
	sprintSize       uint64
	epochSize        uint64
	blockTime        time.Duration
	validators       []string
}

func (p *genesisParams) setFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	flags.StringVar(
		&p.genesisPath,
		dirFlag,
		fmt.Sprintf("./%s", command.DefaultGenesisFileName),
		"the directory for the Polygon Edge genesis data",
	)

	flags.IntVar(
		&p.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain",
	)

	flags.StringVar(
		&p.name,
		nameFlag,
		command.DefaultChainName,
		"the name for the chain",
	)

	flags.StringArrayVar(
		&p.premine,
		premineFlag,
		[]string{},
		fmt.Sprintf(
			"the premined accounts and balances (format: <address>[:<balance>]). Default premined balance: %s",
			command.DefaultPremineBalance,
		),
	)

	flags.Uint64Var(
		&p.blockGasLimit,
		blockGasLimitFlag,
		command.DefaultGenesisGasLimit,
		"the maximum amount of gas used by all transactions in a block",
	)

	flags.StringArrayVar(
		&p.bootnodes,
		command.BootnodeFlag,
		[]string{},
		"multiAddr URL for p2p discovery bootstrap. This flag can be used multiple times",
	)

	flags.StringVar(
		&p.validatorPrefixPath,
		validatorPrefixPathFlag,
		"test-chain-",
		"prefix path for validators",
	)

	// flags.BoolFlag(&flagset.BoolFlag{
	// 	Name:  "bridge",
	// 	Value: &c.bridge,
	// })

	flags.IntVar(
		&p.validatorSetSize,
		validatorSetSizeFlag,
		defaultValidatorSetSize,
		"validator set size",
	)
	flags.Uint64Var(
		&p.epochSize,
		epochSizeFlag,
		defaultEpochSize,
		"epoch size",
	)
	flags.Uint64Var(
		&p.sprintSize,
		sprintSizeFlag,
		defaultSprintSize,
		"sprint size",
	)
	flags.DurationVar(
		&p.blockTime,
		blockTimeFlag,
		defaultBlockTime,
		"block time",
	)
	flags.StringArrayVar(
		&p.validators,
		validatorsFlag,
		[]string{},
		"validators list (format: <address>:<blskey>)",
	)
}

func (p *genesisParams) validateFlags() error {
	// Check if the genesis file already exists
	if generateError := verifyGenesisExistence(p.genesisPath); generateError != nil {
		return errors.New(generateError.GetMessage())
	}

	// Check that the epoch size is correct
	if p.epochSize < 2 {
		// Epoch size must be greater than 1, so new transactions have a chance to be added to a block.
		// Otherwise, every block would be an endblock (meaning it will not have any transactions).
		// Check is placed here to avoid additional parsing if epochSize < 2
		return errInvalidEpochSize
	}

	return nil
}

func (p *genesisParams) getRequiredFlags() []string {
	return []string{} // command.BootnodeFlag,
}

func (p *genesisParams) getPolyBftConfig(validators []GenesisTarget) (*polybft.PolyBFTConfig, error) {
	config := &polybft.PolyBFTConfig{
		// TODO: Genesis, Bridge
		BlockTime:           p.blockTime,
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		ValidatorSetSize:    p.validatorSetSize,
		ValidatorSetAddr:    types.Address(ValidatorSetAddr),
		SidechainBridgeAddr: types.Address(SidechainBridgeAddr),
	}

	if len(p.validators) > 0 {
		for _, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) < 2 {
				continue
			}

			config.Genesis = append(config.Genesis, &polybft.Validator{
				Ecdsa:  types.Address(ethgo.HexToAddress(parts[0])),
				BlsKey: parts[1],
			})
		}
	} else {
		for _, validator := range validators {
			pubKeyMarshalled := validator.Account.Bls.PublicKey().Marshal()

			config.Genesis = append(config.Genesis, &polybft.Validator{
				Ecdsa:  types.Address(validator.Account.Ecdsa.Address()),
				BlsKey: hex.EncodeToString(pubKeyMarshalled),
			})
		}
	}

	return config, nil
}

func (p *genesisParams) GetChainConfig() (*chain.Chain, error) {
	validators, err := ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.validatorPrefixPath)
	if err != nil {
		return nil, err
	}

	polyBftConfig, err := p.getPolyBftConfig(validators)
	if err != nil {
		return nil, err
	}

	extra := polybft.Extra{Validators: &polybft.ValidatorSetDelta{}}

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 0,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  append(make([]byte, 32), extra.MarshalRLPTo(nil)...),
			GasUsed:    command.DefaultGenesisGasUsed,
			Mixhash:    polybft.PolyMixDigest,
		},
		Params: &chain.Params{
			ChainID: p.chainID,
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	// set generic validators as bootnodes if needed
	if len(p.bootnodes) == 0 {
		for i, validator := range validators {
			// /ip4/127.0.0.1/tcp/10001/p2p/16Uiu2HAm9r5oP8Dmfsqbp1w2LdPU4YSFggKvwEmT6aTpWU8c8R13
			bnode := fmt.Sprintf("/ip/%s/tpc/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bnode)
		}
	}

	// Predeploy staking smart contracts
	if err := deployContracts(polyBftConfig, chainConfig.Genesis.Alloc); err != nil {
		return nil, err
	}

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, p.premine); err != nil {
		return nil, err
	}

	return chainConfig, nil
}

func deployContracts(config *polybft.PolyBFTConfig, acc map[types.Address]*chain.GenesisAccount) error {
	// build validator constructor input
	validatorCons := []interface{}{}

	for _, validator := range config.Genesis {
		blsKey, err := hex.DecodeString(validator.BlsKey)
		if err != nil {
			return err
		}

		pubKey, err := bls.UnmarshalPublicKey(blsKey)
		if err != nil {
			return err
		}

		int4, err := pubKey.ToBigInt()
		if err != nil {
			return err
		}

		enc, err := abi.Encode(int4, abi.MustNewType("uint[4]"))
		if err != nil {
			return err
		}

		validatorCons = append(validatorCons, map[string]interface{}{
			"ecdsa": validator.Ecdsa,
			"bls":   enc,
		})
	}

	predefinedContracts := []struct {
		name     string
		input    []interface{}
		expected ethgo.Address
		chain    string
	}{
		{
			// Validator smart contract
			name:     "Validator",
			input:    []interface{}{validatorCons, config.ValidatorSetSize},
			expected: ValidatorSetAddr,
			chain:    "child",
		},
		{
			// Bridge in the sidechain
			name:     "SidechainBridge",
			expected: SidechainBridgeAddr,
			chain:    "child",
		},
		{
			// Target ERC20 token
			name:     "MintERC20",
			expected: sidechainERC20Addr,
			chain:    "child",
		},
		{
			// Bridge wrapper for ERC20 token
			name: "ERC20Bridge",
			input: []interface{}{
				sidechainERC20Addr,
			},
			expected: SidechainERC20BridgeAddr,
			chain:    "child",
		},
	}

	// to call the init in validator smart contract we do not need much more context in the evm object
	// that is why many fields are set as default (as of now).
	for _, contract := range predefinedContracts {
		artifact, err := polybftcontracts.ReadArtifact(contract.chain, contract.name)
		if err != nil {
			return err
		}

		input, err := artifact.DeployInput(contract.input)
		if err != nil {
			return err
		}

		// it is important to keep the same sender so we will always have a deterministic validator address
		// note again that this is only done for testing purposes.
		acc[types.Address(contract.expected)] = &chain.GenesisAccount{
			Code: input,
		}
	}

	return nil
}
