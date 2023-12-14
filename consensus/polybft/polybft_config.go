package polybft

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	ConsensusName              = "polybft"
	minNativeTokenParamsNumber = 3

	defaultNativeTokenName     = "Polygon"
	defaultNativeTokenSymbol   = "MATIC"
	defaultNativeTokenDecimals = uint8(18)
)

var (
	DefaultTokenConfig = &TokenConfig{
		Name:     defaultNativeTokenName,
		Symbol:   defaultNativeTokenSymbol,
		Decimals: defaultNativeTokenDecimals,
	}

	errInvalidTokenParams = errors.New("native token params were not submitted in proper format " +
		"(<name:symbol:decimals count>)")
)

// PolyBFTConfig is the configuration file for the Polybft consensus protocol.
type PolyBFTConfig struct {
	// InitialValidatorSet are the genesis validators
	InitialValidatorSet []*validator.GenesisValidator `json:"initialValidatorSet"`

	// Bridge is the rootchain bridge configuration
	Bridge *BridgeConfig `json:"bridge"`

	// EpochSize is size of epoch
	EpochSize uint64 `json:"epochSize"`

	// EpochReward is assigned to validators for blocks sealing
	EpochReward uint64 `json:"epochReward"`

	// SprintSize is size of sprint
	SprintSize uint64 `json:"sprintSize"`

	// BlockTime is target frequency of blocks production
	BlockTime common.Duration `json:"blockTime"`

	// Governance is the initial governance address
	Governance types.Address `json:"governance"`

	// NativeTokenConfig defines name, symbol and decimal count of the native token
	NativeTokenConfig *TokenConfig `json:"nativeTokenConfig"`

	InitialTrieRoot types.Hash `json:"initialTrieRoot"`

	// MinValidatorSetSize indicates the minimum size of validator set
	MinValidatorSetSize uint64 `json:"minValidatorSetSize"`

	// MaxValidatorSetSize indicates the maximum size of validator set
	MaxValidatorSetSize uint64 `json:"maxValidatorSetSize"`

	// CheckpointInterval indicates the number of blocks after which a new checkpoint is submitted
	CheckpointInterval uint64 `json:"checkpointInterval"`

	// WithdrawalWaitPeriod indicates a number of epochs after which withdrawal can be done from child chain
	WithdrawalWaitPeriod uint64 `json:"withdrawalWaitPeriod"`

	// RewardConfig defines rewards configuration
	RewardConfig *RewardsConfig `json:"rewardConfig"`

	// BlockTimeDrift defines the time slot in which a new block can be created
	BlockTimeDrift uint64 `json:"blockTimeDrift"`

	// BlockTrackerPollInterval specifies interval
	// at which block tracker polls for blocks on a rootchain
	BlockTrackerPollInterval common.Duration `json:"blockTrackerPollInterval"`

	// ProxyContractsAdmin is the address that will have the privilege to change both the proxy
	// implementation address and the admin
	ProxyContractsAdmin types.Address `json:"proxyContractsAdmin"`

	// BladeAdmin is the address that will be the owner of the NativeERC20 mintable token,
	// and StakeManager contract which manages validators
	BladeAdmin types.Address `json:"bladeAdmin"`

	// GovernanceConfig defines on chain governance configuration
	GovernanceConfig *GovernanceConfig `json:"governanceConfig"`

	// StakeTokenAddr represents the stake token contract address
	StakeTokenAddr types.Address `json:"stakeTokenAddr"`
}

// LoadPolyBFTConfig loads chain config from provided path and unmarshals PolyBFTConfig
func LoadPolyBFTConfig(chainConfigFile string) (PolyBFTConfig, error) {
	chainCfg, err := chain.ImportFromFile(chainConfigFile)
	if err != nil {
		return PolyBFTConfig{}, err
	}

	polybftConfig, err := GetPolyBFTConfig(chainCfg.Params)
	if err != nil {
		return PolyBFTConfig{}, err
	}

	return polybftConfig, err
}

// GetPolyBFTConfig deserializes provided chain config and returns PolyBFTConfig
func GetPolyBFTConfig(chainParams *chain.Params) (PolyBFTConfig, error) {
	consensusConfigJSON, err := json.Marshal(chainParams.Engine[ConsensusName])
	if err != nil {
		return PolyBFTConfig{}, err
	}

	var polyBFTConfig PolyBFTConfig
	if err = json.Unmarshal(consensusConfigJSON, &polyBFTConfig); err != nil {
		return PolyBFTConfig{}, err
	}

	return polyBFTConfig, nil
}

// BridgeConfig is the rootchain configuration, needed for bridging
type BridgeConfig struct {
	StateSenderAddr                   types.Address `json:"stateSenderAddress"`
	CheckpointManagerAddr             types.Address `json:"checkpointManagerAddress"`
	ExitHelperAddr                    types.Address `json:"exitHelperAddress"`
	RootERC20PredicateAddr            types.Address `json:"erc20PredicateAddress"`
	ChildMintableERC20PredicateAddr   types.Address `json:"erc20ChildMintablePredicateAddress"`
	RootERC721PredicateAddr           types.Address `json:"erc721PredicateAddress"`
	ChildMintableERC721PredicateAddr  types.Address `json:"erc721ChildMintablePredicateAddress"`
	RootERC1155PredicateAddr          types.Address `json:"erc1155PredicateAddress"`
	ChildMintableERC1155PredicateAddr types.Address `json:"erc1155ChildMintablePredicateAddress"`
	ChildERC20Addr                    types.Address `json:"childERC20Address"`
	ChildERC721Addr                   types.Address `json:"childERC721Address"`
	ChildERC1155Addr                  types.Address `json:"childERC1155Address"`
	// only populated if stake-manager-deploy command is executed, and used for e2e tests
	BLSAddress     types.Address `json:"blsAddr"`
	BN256G2Address types.Address `json:"bn256G2Addr"`

	JSONRPCEndpoint         string                   `json:"jsonRPCEndpoint"`
	EventTrackerStartBlocks map[types.Address]uint64 `json:"eventTrackerStartBlocks"`
}

func (p *PolyBFTConfig) IsBridgeEnabled() bool {
	return p.Bridge != nil
}

// RootchainConfig contains rootchain metadata (such as JSON RPC endpoint and contract addresses)
type RootchainConfig struct {
	JSONRPCAddr string

	StateSenderAddress                   types.Address
	CheckpointManagerAddress             types.Address
	BLSAddress                           types.Address
	BN256G2Address                       types.Address
	ExitHelperAddress                    types.Address
	RootERC20PredicateAddress            types.Address
	ChildMintableERC20PredicateAddress   types.Address
	ChildERC20Address                    types.Address
	RootERC721PredicateAddress           types.Address
	ChildMintableERC721PredicateAddress  types.Address
	ChildERC721Address                   types.Address
	RootERC1155PredicateAddress          types.Address
	ChildMintableERC1155PredicateAddress types.Address
	ChildERC1155Address                  types.Address
}

// ToBridgeConfig creates BridgeConfig instance
func (r *RootchainConfig) ToBridgeConfig() *BridgeConfig {
	return &BridgeConfig{
		JSONRPCEndpoint: r.JSONRPCAddr,

		StateSenderAddr:                   r.StateSenderAddress,
		CheckpointManagerAddr:             r.CheckpointManagerAddress,
		ExitHelperAddr:                    r.ExitHelperAddress,
		RootERC20PredicateAddr:            r.RootERC20PredicateAddress,
		ChildMintableERC20PredicateAddr:   r.ChildMintableERC20PredicateAddress,
		RootERC721PredicateAddr:           r.RootERC721PredicateAddress,
		ChildMintableERC721PredicateAddr:  r.ChildMintableERC721PredicateAddress,
		RootERC1155PredicateAddr:          r.RootERC1155PredicateAddress,
		ChildMintableERC1155PredicateAddr: r.ChildMintableERC1155PredicateAddress,
		ChildERC20Addr:                    r.ChildERC20Address,
		ChildERC721Addr:                   r.ChildERC721Address,
		ChildERC1155Addr:                  r.ChildERC1155Address,
		BLSAddress:                        r.BLSAddress,
		BN256G2Address:                    r.BN256G2Address,
	}
}

// TokenConfig is the configuration of native token used by edge network
type TokenConfig struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals uint8  `json:"decimals"`
}

func ParseRawTokenConfig(rawConfig string) (*TokenConfig, error) {
	if rawConfig == "" {
		return DefaultTokenConfig, nil
	}

	params := strings.Split(rawConfig, ":")
	if len(params) < minNativeTokenParamsNumber {
		return nil, errInvalidTokenParams
	}

	// name
	name := strings.TrimSpace(params[0])
	if name == "" {
		return nil, errInvalidTokenParams
	}

	// symbol
	symbol := strings.TrimSpace(params[1])
	if symbol == "" {
		return nil, errInvalidTokenParams
	}

	// decimals
	decimals, err := strconv.ParseUint(strings.TrimSpace(params[2]), 10, 8)
	if err != nil || decimals > math.MaxUint8 {
		return nil, errInvalidTokenParams
	}

	return &TokenConfig{
		Name:     name,
		Symbol:   symbol,
		Decimals: uint8(decimals),
	}, nil
}

type RewardsConfig struct {
	// TokenAddress is the address of reward token on child chain
	TokenAddress types.Address

	// WalletAddress is the address of reward wallet on child chain
	WalletAddress types.Address

	// WalletAmount is the amount of tokens in reward wallet
	WalletAmount *big.Int
}

func (r *RewardsConfig) MarshalJSON() ([]byte, error) {
	raw := &rewardsConfigRaw{
		TokenAddress:  r.TokenAddress,
		WalletAddress: r.WalletAddress,
		WalletAmount:  common.EncodeBigInt(r.WalletAmount),
	}

	return json.Marshal(raw)
}

func (r *RewardsConfig) UnmarshalJSON(data []byte) error {
	var (
		raw rewardsConfigRaw
		err error
	)

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.TokenAddress = raw.TokenAddress
	r.WalletAddress = raw.WalletAddress

	r.WalletAmount, err = common.ParseUint256orHex(raw.WalletAmount)
	if err != nil {
		return err
	}

	return nil
}

type GovernanceConfig struct {
	// VotingDelay indicates number of blocks after proposal is submitted before voting starts
	VotingDelay *big.Int
	// VotingPeriod indicates number of blocks that the voting period for a proposal lasts
	VotingPeriod *big.Int
	// ProposalThreshold indicates number of vote tokens required in order for a voter to become a proposer
	ProposalThreshold *big.Int
	// ProposalQuorumPercentage is the percentage of total validator stake needed for a
	// governance proposal to be accepted
	ProposalQuorumPercentage uint64
	// ChildGovernorAddr is the address of ChildGovernor contract
	ChildGovernorAddr types.Address
	// ChildTimelockAddr is the address of ChildTimelock contract
	ChildTimelockAddr types.Address
	// NetworkParamsAddr is the address of NetworkParams contract
	NetworkParamsAddr types.Address
	// ForkParamsAddr is the address of ForkParams contract
	ForkParamsAddr types.Address
}

func (g *GovernanceConfig) MarshalJSON() ([]byte, error) {
	raw := &governanceConfigRaw{
		VotingDelay:              common.EncodeBigInt(g.VotingDelay),
		VotingPeriod:             common.EncodeBigInt(g.VotingPeriod),
		ProposalThreshold:        common.EncodeBigInt(g.ProposalThreshold),
		ProposalQuorumPercentage: g.ProposalQuorumPercentage,
		ChildGovernorAddr:        g.ChildGovernorAddr,
		ChildTimelockAddr:        g.ChildTimelockAddr,
		NetworkParamsAddr:        g.NetworkParamsAddr,
		ForkParamsAddr:           g.ForkParamsAddr,
	}

	return json.Marshal(raw)
}

func (g *GovernanceConfig) UnmarshalJSON(data []byte) error {
	var (
		raw governanceConfigRaw
		err error
	)

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	g.VotingDelay, err = common.ParseUint256orHex(raw.VotingDelay)
	if err != nil {
		return err
	}

	g.VotingPeriod, err = common.ParseUint256orHex(raw.VotingPeriod)
	if err != nil {
		return err
	}

	g.ProposalThreshold, err = common.ParseUint256orHex(raw.ProposalThreshold)
	if err != nil {
		return err
	}

	g.ProposalQuorumPercentage = raw.ProposalQuorumPercentage
	g.ChildGovernorAddr = raw.ChildGovernorAddr
	g.ChildTimelockAddr = raw.ChildTimelockAddr
	g.NetworkParamsAddr = raw.NetworkParamsAddr
	g.ForkParamsAddr = raw.ForkParamsAddr

	return nil
}

type governanceConfigRaw struct {
	VotingDelay              *string       `json:"votingDelay"`
	VotingPeriod             *string       `json:"votingPeriod"`
	ProposalThreshold        *string       `json:"proposalThreshold"`
	ProposalQuorumPercentage uint64        `json:"proposalQuorumPercentage"`
	ChildGovernorAddr        types.Address `json:"childGovernorAddr"`
	ChildTimelockAddr        types.Address `json:"childTimelockAddr"`
	NetworkParamsAddr        types.Address `json:"networkParamsAddr"`
	ForkParamsAddr           types.Address `json:"forkParamsAddr"`
}

type rewardsConfigRaw struct {
	TokenAddress  types.Address `json:"rewardTokenAddress"`
	WalletAddress types.Address `json:"rewardWalletAddress"`
	WalletAmount  *string       `json:"rewardWalletAmount"`
}
