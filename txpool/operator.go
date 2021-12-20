package txpool

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// Status implements the GRPC status endpoint. Returns the number of transactions in the pool
func (t *TxPool) Status(ctx context.Context, req *empty.Empty) (*proto.TxnPoolStatusResp, error) {
	resp := &proto.TxnPoolStatusResp{
		Length: t.pendingQueue.Length(),
	}

	return resp, nil
}

// AddTxn adds a local transaction to the pool
func (t *TxPool) AddTxn(ctx context.Context, raw *proto.AddTxnReq) (*empty.Empty, error) {
	if raw.Raw == nil {
		return nil, fmt.Errorf("transaction's field raw is empty")
	}

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
