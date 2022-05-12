package ibft

import (
	"fmt"

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

// putIbftExtraValidators is a helper method that adds validators to the extra field in the header
func putIbftExtraValidators(h *types.Header, validators []types.Address) {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < IstanbulExtraVanity {
		extra = append(extra, zeroBytes[:IstanbulExtraVanity-len(extra)]...)
	} else {
		extra = extra[:IstanbulExtraVanity]
	}

	ibftExtra := &IstanbulExtra{
		Validators:    validators,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	extra = ibftExtra.MarshalRLPTo(extra)
	h.ExtraData = extra
}

// PutIbftExtra sets the extra data field in the header to the passed in istanbul extra data
func PutIbftExtra(h *types.Header, istanbulExtra *IstanbulExtra) error {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < IstanbulExtraVanity {
		extra = append(extra, zeroBytes[:IstanbulExtraVanity-len(extra)]...)
	} else {
		extra = extra[:IstanbulExtraVanity]
	}

	data := istanbulExtra.MarshalRLPTo(nil)
	extra = append(extra, data...)
	h.ExtraData = extra

	return nil
}

// getIbftExtra returns the istanbul extra data field from the passed in header
func getIbftExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, fmt.Errorf("wrong extra size: %d", len(h.ExtraData))
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

// IstanbulExtra defines the structure of the extra field for Istanbul
type IstanbulExtra struct {
	Validators    []types.Address
	Seal          []byte
	CommittedSeal [][]byte
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
		vals.Set(ar.NewBytes(a.Bytes()))
	}

	vv.Set(vals)

	// Seal
	if len(i.Seal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(i.Seal))
	}

	// CommittedSeal
	if len(i.CommittedSeal) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		committed := ar.NewArray()
		for _, a := range i.CommittedSeal {
			if len(a) == 0 {
				vv.Set(ar.NewNull())
			} else {
				committed.Set(ar.NewBytes(a))
			}
		}
		vv.Set(committed)
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

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode istambul extra, expected 3 but found %d", len(elems))
	}

	// Validators
	{
		vals, err := elems[0].GetElems()
		if err != nil {
			return fmt.Errorf("list expected for validators")
		}
		i.Validators = make([]types.Address, len(vals))
		for indx, val := range vals {
			if err = val.GetAddr(i.Validators[indx][:]); err != nil {
				return err
			}
		}
	}

	// Seal
	{
		if i.Seal, err = elems[1].GetBytes(i.Seal); err != nil {
			return err
		}
	}

	// Committed
	{
		vals, err := elems[2].GetElems()
		if err != nil {
			return fmt.Errorf("list expected for committed")
		}
		i.CommittedSeal = make([][]byte, len(vals))
		for indx, val := range vals {
			if i.CommittedSeal[indx], err = val.GetBytes(i.CommittedSeal[indx]); err != nil {
				return err
			}
		}
	}

	return nil
}
