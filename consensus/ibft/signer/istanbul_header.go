package signer

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

type IstanbulHeader types.Header

func (h *IstanbulHeader) InitExtra(
	vals validators.Validators,
	seals Seals,
	signer Signer,
) {

}
