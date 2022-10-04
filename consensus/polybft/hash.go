package polybft

import "github.com/0xPolygon/polygon-edge/types"

func init() {
	types.NewMixHash(PolyMixDigest, polyBFTHeaderHash)
}

// polyBFTHeaderHash defines the custom implementation for getting the header hash,
// because of the extraData field
func polyBFTHeaderHash(h *types.Header) types.Hash {
	hh := h.Copy()
	// when hashing the block for signing we have to remove from
	// the extra field the seal and committed seal items
	extra, err := GetIbftExtraClean(h.ExtraData)
	if err != nil {
		// TODO: this should not happen but we have to handle it somehow
		panic(err)
	}
	// override extra data without seals and committed seal items
	hh.ExtraData = extra

	return types.HeaderHash(hh)
}
