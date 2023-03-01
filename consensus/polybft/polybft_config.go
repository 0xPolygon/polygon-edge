package polybft

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// PolyBFTConfig is the configuration file for the Polybft consensus protocol.
type PolyBFTConfig struct {
	// InitialValidatorSet are the genesis validators
	InitialValidatorSet []*Validator `json:"initialValidatorSet"`

	// Bridge is the rootchain bridge configuration
	Bridge *BridgeConfig `json:"bridge"`

	// EpochSize is size of epoch
	EpochSize uint64 `json:"epochSize"`

	// EpochReward is assigned to validators for blocks sealing
	EpochReward uint64 `json:"epochReward"`

	// SprintSize is size of sprint
	SprintSize uint64 `json:"sprintSize"`

	// BlockTime is target frequency of blocks production
	BlockTime time.Duration `json:"blockTime"`

	// Governance is the initial governance address
	Governance types.Address `json:"governance"`

	// TODO: Remove these two addresses as they are hardcoded and known in advance
	// Address of the system contracts, as of now (testing) this is populated automatically during genesis
	ValidatorSetAddr  types.Address `json:"validatorSetAddr"`
	StateReceiverAddr types.Address `json:"stateReceiverAddr"`
}

// GetPolyBFTConfig deserializes provided chain config and returns PolyBFTConfig
func GetPolyBFTConfig(chainConfig *chain.Chain) (PolyBFTConfig, error) {
	consensusConfigJSON, err := json.Marshal(chainConfig.Params.Engine["polybft"])
	if err != nil {
		return PolyBFTConfig{}, err
	}

	var polyBFTConfig PolyBFTConfig
	err = json.Unmarshal(consensusConfigJSON, &polyBFTConfig)

	if err != nil {
		return PolyBFTConfig{}, err
	}

	return polyBFTConfig, nil
}

// BridgeConfig is the rootchain bridge configuration
type BridgeConfig struct {
	BridgeAddr             types.Address `json:"stateSenderAddr"`
	CheckpointAddr         types.Address `json:"checkpointAddr"`
	RootERC20PredicateAddr types.Address `json:"rootERC20PredicateAddr"`
	RootNativeERC20Addr    types.Address `json:"rootNativeERC20Addr"`
	JSONRPCEndpoint        string        `json:"jsonRPCEndpoint"`
}

func (p *PolyBFTConfig) IsBridgeEnabled() bool {
	return p.Bridge != nil
}

// Validator represents public information about validator accounts which are the part of genesis
type Validator struct {
	Address       types.Address
	BlsPrivateKey *bls.PrivateKey
	BlsKey        string
	BlsSignature  string
	Balance       *big.Int
	NodeID        string
}

type validatorRaw struct {
	Address      types.Address `json:"address"`
	BlsKey       string        `json:"blsKey"`
	BlsSignature string        `json:"blsSignature"`
	Balance      *string       `json:"balance"`
	NodeID       string        `json:"nodeId"`
}

func (v *Validator) MarshalJSON() ([]byte, error) {
	raw := &validatorRaw{Address: v.Address, BlsKey: v.BlsKey, NodeID: v.NodeID, BlsSignature: v.BlsSignature}
	raw.Balance = types.EncodeBigInt(v.Balance)

	return json.Marshal(raw)
}

func (v *Validator) UnmarshalJSON(data []byte) error {
	var raw validatorRaw

	var err error

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.Address = raw.Address
	v.BlsKey = raw.BlsKey
	v.BlsSignature = raw.BlsSignature
	v.NodeID = raw.NodeID
	v.Balance, err = types.ParseUint256orHex(raw.Balance)

	if err != nil {
		return err
	}

	return nil
}

// UnmarshalBLSPublicKey unmarshals the hex encoded BLS public key
func (v *Validator) UnmarshalBLSPublicKey() (*bls.PublicKey, error) {
	decoded, err := hex.DecodeString(v.BlsKey)
	if err != nil {
		return nil, err
	}

	return bls.UnmarshalPublicKey(decoded)
}

// UnmarshalBLSSignature unmarshals the hex encoded BLS signature
func (v *Validator) UnmarshalBLSSignature() (*bls.Signature, error) {
	decoded, err := hex.DecodeString(v.BlsSignature)
	if err != nil {
		return nil, err
	}

	return bls.UnmarshalSignature(decoded)
}

// ToValidatorInitAPIBinding converts Validator to instance of contractsapi.ValidatorInit
func (v Validator) ToValidatorInitAPIBinding() (*contractsapi.ValidatorInit, error) {
	blsSignature, err := v.UnmarshalBLSSignature()
	if err != nil {
		return nil, err
	}

	signBigInts, err := blsSignature.ToBigInt()
	if err != nil {
		return nil, err
	}

	pubKey, err := v.UnmarshalBLSPublicKey()
	if err != nil {
		return nil, err
	}

	return &contractsapi.ValidatorInit{
		Addr:      v.Address,
		Pubkey:    pubKey.ToBigInt(),
		Signature: signBigInts,
		Stake:     new(big.Int).Set(v.Balance),
	}, nil
}

// ToValidatorMetadata creates ValidatorMetadata instance
func (v *Validator) ToValidatorMetadata() (*ValidatorMetadata, error) {
	blsKey, err := v.UnmarshalBLSPublicKey()
	if err != nil {
		return nil, err
	}

	metadata := &ValidatorMetadata{
		Address:     v.Address,
		BlsKey:      blsKey,
		VotingPower: new(big.Int).Set(v.Balance),
	}

	return metadata, nil
}

// RootchainConfig contains information about rootchain contract addresses
// as well as rootchain admin account address
type RootchainConfig struct {
	StateSenderAddress        types.Address `json:"stateSenderAddress"`
	CheckpointManagerAddress  types.Address `json:"checkpointManagerAddress"`
	BLSAddress                types.Address `json:"blsAddress"`
	BN256G2Address            types.Address `json:"bn256G2Address"`
	ExitHelperAddress         types.Address `json:"exitHelperAddress"`
	RootERC20PredicateAddress types.Address `json:"rootERC20PredicateAddress"`
	RootNativeERC20Address    types.Address `json:"rootNativeERC20Address"`
	ERC20TemplateAddress      types.Address `json:"erc20TemplateAddress"`
}

// ToBridgeConfig creates BridgeConfig instance
func (r *RootchainConfig) ToBridgeConfig() *BridgeConfig {
	return &BridgeConfig{
		BridgeAddr:             r.StateSenderAddress,
		CheckpointAddr:         r.CheckpointManagerAddress,
		RootERC20PredicateAddr: r.RootERC20PredicateAddress,
		RootNativeERC20Addr:    r.RootNativeERC20Address,
	}
}

// Manifest holds metadata, such as genesis validators and rootchain configuration
type Manifest struct {
	GenesisValidators []*Validator     `json:"validators"`
	RootchainConfig   *RootchainConfig `json:"rootchain"`
	ChainID           int64            `json:"chainID"`
}

// LoadManifest deserializes Manifest instance
func LoadManifest(metadataFile string) (*Manifest, error) {
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil, err
	}

	var manifest Manifest

	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// Save marshals RootchainManifest instance to json and persists it to given location
func (m *Manifest) Save(manifestPath string) error {
	data, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal rootchain manifest to JSON: %w", err)
	}

	if err := common.SaveFileSafe(filepath.Clean(manifestPath), data, 0660); err != nil {
		return fmt.Errorf("failed to save rootchain manifest file: %w", err)
	}

	return nil
}
