package validator

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
)

// GenesisValidator represents public information about validator accounts which are the part of genesis
type GenesisValidator struct {
	Address       types.Address
	BlsPrivateKey *bls.PrivateKey
	BlsKey        string
	Balance       *big.Int
	Stake         *big.Int
	MultiAddr     string
}

type genesisValidatorRaw struct {
	Address   types.Address `json:"address"`
	BlsKey    string        `json:"blsKey"`
	Balance   *string       `json:"balance"`
	Stake     *string       `json:"stake"`
	MultiAddr string        `json:"multiAddr"`
}

func (v *GenesisValidator) MarshalJSON() ([]byte, error) {
	raw := &genesisValidatorRaw{Address: v.Address, BlsKey: v.BlsKey, MultiAddr: v.MultiAddr}
	raw.Balance = types.EncodeBigInt(v.Balance)
	raw.Stake = types.EncodeBigInt(v.Stake)

	return json.Marshal(raw)
}

func (v *GenesisValidator) UnmarshalJSON(data []byte) error {
	var (
		raw genesisValidatorRaw
		err error
	)

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.Address = raw.Address
	v.BlsKey = raw.BlsKey
	v.MultiAddr = raw.MultiAddr

	v.Balance, err = types.ParseUint256orHex(raw.Balance)
	if err != nil {
		return err
	}

	v.Stake, err = types.ParseUint256orHex(raw.Stake)
	if err != nil {
		return err
	}

	return nil
}

// UnmarshalBLSPublicKey unmarshals the hex encoded BLS public key
func (v *GenesisValidator) UnmarshalBLSPublicKey() (*bls.PublicKey, error) {
	decoded, err := hex.DecodeString(v.BlsKey)
	if err != nil {
		return nil, err
	}

	return bls.UnmarshalPublicKey(decoded)
}

// ToValidatorMetadata creates ValidatorMetadata instance
func (v *GenesisValidator) ToValidatorMetadata() (*ValidatorMetadata, error) {
	blsKey, err := v.UnmarshalBLSPublicKey()
	if err != nil {
		return nil, err
	}

	metadata := &ValidatorMetadata{
		Address:     v.Address,
		BlsKey:      blsKey,
		VotingPower: new(big.Int).Set(v.Stake),
		IsActive:    true,
	}

	return metadata, nil
}

// String implements fmt.Stringer interface
func (v *GenesisValidator) String() string {
	return fmt.Sprintf("Address=%s; Balance=%d; Stake=%d; P2P Multi addr=%s; BLS Key=%s;",
		v.Address, v.Balance, v.Stake, v.MultiAddr, v.BlsKey)
}
