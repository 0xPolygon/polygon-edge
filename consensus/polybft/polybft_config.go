package polybft

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
)

const PolyBFTConsensusName = "polybft"

// PolyBFTConfig is the configuration file for the Polybft consensus protocol.
type PolyBFTConfig struct {
	Manifest *Manifest `json:"manifest"`
	// TODO: @Stefan-Ethernal remove
	// InitialValidatorSet, Bridge,
	// ValidatorSetAddr and StateReceiverAddr
	InitialValidatorSet []*Validator  `json:"initialValidatorSet"`
	Bridge              *BridgeConfig `json:"bridge"`

	ValidatorSetSize int `json:"validatorSetSize"`

	// Address of the system contracts, as of now (testing) this is populated automatically during genesis
	ValidatorSetAddr  types.Address `json:"validatorSetAddr"`
	StateReceiverAddr types.Address `json:"stateReceiverAddr"`

	// size of the epoch and sprint
	EpochSize  uint64 `json:"epochSize"`
	SprintSize uint64 `json:"sprintSize"`

	BlockTime time.Duration `json:"blockTime"`

	// Governance is the initial governance address
	Governance types.Address `json:"governance"`
}

// GetPolyBFTConfig deserializes provided chain config and returns PolyBFTConfig
func GetPolyBFTConfig(chainConfig *chain.Chain) (PolyBFTConfig, error) {
	consensusConfigJSON, err := json.Marshal(chainConfig.Params.Engine[PolyBFTConsensusName])
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
	BridgeAddr      types.Address `json:"stateSenderAddr"`
	CheckpointAddr  types.Address `json:"checkpointAddr"`
	JSONRPCEndpoint string        `json:"jsonRPCEndpoint"`
}

func (p *PolyBFTConfig) IsBridgeEnabled() bool {
	return p.Bridge != nil
}

// Validator represents public information about validator accounts which are the part of genesis
type Validator struct {
	Address types.Address `json:"address"`
	BlsKey  string        `json:"blsKey"`
	Balance *big.Int      `json:"balance"`
	NodeID  string        `json:"nodeId"`
}

type validatorRaw struct {
	Address types.Address `json:"address"`
	BlsKey  string        `json:"blsKey"`
	Balance *string       `json:"balance"`
}

func (v *Validator) MarshalJSON() ([]byte, error) {
	raw := &validatorRaw{Address: v.Address, BlsKey: v.BlsKey}
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

// DebugConfig is a struct used for test configuration in init genesis
type DebugConfig struct {
	ValidatorSetSize uint64 `json:"validatorSetSize"`
}

// RootchainConfig contains information about rootchain contract addresses
// as well as rootchain admin account address
type RootchainConfig struct {
	StateSenderAddress       types.Address `json:"stateSenderAddress"`
	CheckpointManagerAddress types.Address `json:"checkpointManagerAddress"`
	BLSAddress               types.Address `json:"blsAddress"`
	BN256G2Address           types.Address `json:"bn256G2Address"`
	AdminAddress             types.Address `json:"adminAddress"`
}

// Manifest holds metadata, such as genesis validators and rootchain configuration
type Manifest struct {
	GenesisValidators []*Validator     `json:"validators"`
	RootchainConfig   *RootchainConfig `json:"rootchain"`
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

	if err := os.WriteFile(manifestPath, data, os.ModePerm); err != nil {
		return fmt.Errorf("failed to save rootchain manifest file: %w", err)
	}

	return nil
}
