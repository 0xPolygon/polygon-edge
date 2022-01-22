package proto

import "github.com/0xPolygon/polygon-edge/types"

// DecodeHashes decode to types Hash in the request
func (h *HashRequest) DecodeHashes() ([]types.Hash, error) {
	resp := []types.Hash{}

	for _, h := range h.Hash {
		hh := types.Hash{}
		if err := hh.UnmarshalText([]byte(h)); err != nil {
			return nil, err
		}

		resp = append(resp, hh)
	}

	return resp, nil
}
