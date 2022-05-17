package ibft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	// IstanbulDigest represents a hash of "Istanbul practical byzantine fault tolerance"
	// to identify whether the block is from Istanbul consensus engine
	IstanbulDigest = types.StringToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	// IstanbulExtraVanity represents a fixed number of extra-data bytes reserved for proposer vanity
	IstanbulExtraVanity = 32

	// IstanbulExtraSeal represents the fixed number of extra-data bytes reserved for proposer seal
	IstanbulExtraSeal = 65
)

var zeroBytes = make([]byte, 32)

// initIbftExtra initializes ExtraData in Header for IBFT
func initIbftExtra(h *types.Header, validators [][]byte, parentCommittedSeal Sealer, bls bool) {
	var committedSeal Sealer = new(BLSSeal)
	if bls {
		committedSeal = new(BLSSeal)
	}

	putIbftExtra(h, &IstanbulExtra{
		Validators:          validators,
		Seal:                []byte{},
		CommittedSeal:       committedSeal,
		ParentCommittedSeal: parentCommittedSeal,
	})
}

// putIbftExtra sets the IBFT extra data field into the header
func putIbftExtra(h *types.Header, istanbulExtra *IstanbulExtra) {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < IstanbulExtraVanity {
		extra = append(extra, zeroBytes[:IstanbulExtraVanity-len(extra)]...)
	} else {
		extra = extra[:IstanbulExtraVanity]
	}

	h.ExtraData = istanbulExtra.MarshalRLPTo(extra)
}

// getIbftExtra extracts the istanbul extra data from the given header
func getIbftExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(h.ExtraData),
		)
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		// TODO
		CommittedSeal:       &BLSSeal{},
		ParentCommittedSeal: &BLSSeal{},
	}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

// unpackSealFromIbftExtra extracts Seal from the istanbul extra of the given Header
func unpackSealFromIbftExtra(h *types.Header) ([]byte, error) {
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.Seal, nil
}

func unpackValidatorsFromIbftExtra(h *types.Header) ([][]byte, error) {
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.Validators, nil
}

// unpackCommittedSealFromIbftExtra extracts CommittedSeal from the istanbul extra of the given Header
func unpackCommittedSealFromIbftExtra(h *types.Header) (Sealer, error) {
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.CommittedSeal, nil
}

// unpackParentCommittedSealFromIbftExtra extracts ParentCommittedSeal from the istanbul extra of the given Header
func unpackParentCommittedSealFromIbftExtra(h *types.Header) (Sealer, error) {
	extra, err := getIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.ParentCommittedSeal, nil
}

// packFieldIntoIbftExtra is a helper function to update fields in the istanbul Extra of the given header
func packFieldIntoIbftExtra(h *types.Header, updateFn func(*IstanbulExtra)) error {
	extra, err := getIbftExtra(h)
	if err != nil {
		return err
	}

	updateFn(extra)

	putIbftExtra(h, extra)

	return nil
}

// packSealIntoIbftExtra sets the seal to Seal field in istanbul extra of the given header
func packSealIntoIbftExtra(h *types.Header, seal []byte) error {
	return packFieldIntoIbftExtra(h, func(extra *IstanbulExtra) {
		extra.Seal = seal
	})
}

// packCommittedSealIntoIbftExtra sets the seals to CommittedSeal field in istanbul extra of the given header
func packCommittedSealIntoIbftExtra(h *types.Header, seals Sealer) error {
	return packFieldIntoIbftExtra(h, func(extra *IstanbulExtra) {
		extra.CommittedSeal = seals
	})
}

// filterIbftExtraForHash clears unnecessary fields in istanbul Extra of the given header for hash calculation
func filterIbftExtraForHash(h *types.Header, bls bool) error {
	extra, err := getIbftExtra(h)
	if err != nil {
		return err
	}

	// This will effectively remove the Seal and Committed Seal fields,
	// while keeping proposer vanity and validator set
	// because extra.Validators, extra.ParentCommittedSeal is what we got from `h` in the first place.
	initIbftExtra(h, extra.Validators, extra.ParentCommittedSeal, bls)

	return nil
}

// IstanbulExtra defines the structure of the extra field for Istanbul
type IstanbulExtra struct {
	Validators          [][]byte // ECDSA: Address, BLS: PubKey
	Seal                []byte
	CommittedSeal       Sealer
	ParentCommittedSeal Sealer
}

type Sealer interface {
	MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}

type SerializedSeal [][]byte

type BLSSeal struct {
	Bitmap    *big.Int
	Signature []byte
}

// MarshalRLPTo defines the marshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(i.MarshalRLPWith, dst)
}

// MarshalRLPWith defines the marshal function implementation for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// Validators
	vals := ar.NewArray()
	for _, a := range i.Validators {
		vals.Set(ar.NewBytes(a))
	}

	vv.Set(vals)

	// Seal
	if len(i.Seal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(i.Seal))
	}

	// CommittedSeal
	vv.Set(i.CommittedSeal.MarshalRLPWith(ar))

	// ParentCommittedSeal
	if i.ParentCommittedSeal != nil {
		vv.Set(i.ParentCommittedSeal.MarshalRLPWith(ar))
	}

	return vv
}

// UnmarshalRLP defines the unmarshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(i.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom defines the unmarshal implementation for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 3 && num != 4 {
		return fmt.Errorf("not enough elements to decode istambul extra, expected 3 or 4 but found %d", num)
	}

	// Validators
	{
		vals, err := elems[0].GetElems()
		if err != nil {
			return fmt.Errorf("mismatch of RLP type for Validators, expected list but found %s", elems[0].Type())
		}

		i.Validators = make([][]byte, len(vals))
		for indx, val := range vals {
			if i.Validators[indx], err = val.GetBytes(i.Validators[indx]); err != nil {
				return err
			}
		}
	}

	// Seal
	{
		if i.Seal, err = elems[1].GetBytes(i.Seal); err != nil {
			return fmt.Errorf("failed to decode Seal: %w", err)
		}
	}

	// Committed
	if err := i.CommittedSeal.UnmarshalRLPFrom(p, elems[2]); err != nil {
		return err
	}

	// LastCommitted
	if len(elems) >= 4 {
		if err := i.ParentCommittedSeal.UnmarshalRLPFrom(p, elems[3]); err != nil {
			return err
		}
	}

	return nil
}

func (s *SerializedSeal) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*s) == 0 {
		return ar.NewNullArray()
	}

	committed := ar.NewArray()
	for _, a := range *s {
		if len(a) == 0 {
			committed.Set(ar.NewNull())
		} else {
			committed.Set(ar.NewBytes(a))
		}
	}

	return committed
}

func (s *SerializedSeal) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", v.Type())
	}

	(*s) = make([][]byte, len(vals))

	for indx, val := range vals {
		if (*s)[indx], err = val.GetBytes((*s)[indx]); err != nil {
			return err
		}
	}

	return nil
}

func (s *BLSSeal) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	x := ar.NewArray()

	if s.Bitmap == nil {
		x.Set(ar.NewNull())
	} else {
		x.Set(ar.NewBytes(s.Bitmap.Bytes()))
	}

	if s.Signature == nil {
		x.Set(ar.NewNull())
	} else {
		x.Set(ar.NewBytes(s.Signature))
	}

	return x
}

func (s *BLSSeal) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", v.Type())
	}
	if len(vals) == 0 {
		// FIXME?: Empty
		return nil
	}

	if len(vals) != 2 {
		return fmt.Errorf("mismatch of RLP type for AggregatedCommittedSeal")
	}

	var rawBitMap []byte

	rawBitMap, err = vals[0].GetBytes(rawBitMap)
	if err != nil {
		return err
	}

	s.Bitmap = new(big.Int).SetBytes(rawBitMap)
	if s.Signature, err = vals[1].GetBytes(s.Signature); err != nil {
		return err
	}

	return nil
}
