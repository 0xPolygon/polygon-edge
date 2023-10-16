package validator

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// GenesisValidator represents public information about validator accounts which are the part of genesis
type GenesisValidator struct {
	Address   types.Address
	BlsKey    string
	Stake     *big.Int
	MultiAddr string
}

type genesisValidatorRaw struct {
	Address   types.Address `json:"address"`
	BlsKey    string        `json:"blsKey"`
	Stake     *string       `json:"stake"`
	MultiAddr string        `json:"multiAddr"`
}

func (v *GenesisValidator) MarshalJSON() ([]byte, error) {
	raw := &genesisValidatorRaw{Address: v.Address, BlsKey: v.BlsKey, MultiAddr: v.MultiAddr}
	raw.Stake = common.EncodeBigInt(v.Stake)

	return json.Marshal(raw)
}

func (v *GenesisValidator) UnmarshalJSON(data []byte) (err error) {
	var raw genesisValidatorRaw

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.Address = raw.Address
	v.BlsKey = raw.BlsKey
	v.MultiAddr = raw.MultiAddr

	v.Stake, err = common.ParseUint256orHex(raw.Stake)

	return err
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
	return fmt.Sprintf("Address=%s; Stake=%d; P2P Multi addr=%s; BLS Key=%s;",
		v.Address, v.Stake, v.MultiAddr, v.BlsKey)
}
