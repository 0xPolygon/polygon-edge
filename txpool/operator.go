package txpool

import (
	"context"

	"github.com/0xPolygon/minimal/txpool/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
)

// Status implements the GRPC status endpoint. Returns the number of transactions in the pool
func (t *TxPool) Status(ctx context.Context, req *empty.Empty) (*proto.TxnPoolStatusResp, error) {
	resp := &proto.TxnPoolStatusResp{
		Length: t.sorted.Length(),
	}

	return resp, nil
}

// AddTxn adds a local transaction to the pool
func (t *TxPool) AddTxn(ctx context.Context, raw *proto.AddTxnReq) (*empty.Empty, error) {
	txn := new(types.Transaction)
	if err := txn.UnmarshalRLP(raw.Raw.Value); err != nil {
		return nil, err
	}

	if raw.From != "" {
		from := types.Address{}
		if err := from.UnmarshalText([]byte(raw.From)); err != nil {
			return nil, err
		}
		txn.From = from
	}

	if err := t.AddTx(txn); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Subscribe implements the operator endpoint. It subscribes to new events in the tx pool
func (t *TxPool) Subscribe(req *empty.Empty, stream proto.TxnPoolOperator_SubscribeServer) error {
	// TODO
	return nil
}