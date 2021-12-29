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
	t.LockPromoted(false)
	defer t.UnlockPromoted()

	resp := &proto.TxnPoolStatusResp{
		Length: t.promoted.length(),
	}

	return resp, nil
}

// AddTxn adds a local transaction to the pool
func (t *TxPool) AddTxn(ctx context.Context, raw *proto.AddTxnReq) (*proto.AddTxnResp, error) {
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

	return &proto.AddTxnResp{
		TxHash: txn.Hash.String(),
	}, nil
}

// Subscribe implements the operator endpoint. It subscribes to new events in the tx pool
func (t *TxPool) Subscribe(
	request *proto.SubscribeRequest,
	stream proto.TxnPoolOperator_SubscribeServer,
) error {
	subscription := t.eventManager.subscribe(request.Types)

	teardown := func() error {
		t.eventManager.cancelSubscription(subscription.subscriptionID)

		return nil
	}

	for {
		select {
		case event, more := <-subscription.subscriptionChannel:
			if !more {
				return nil
			}

			if sendErr := stream.Send(event); sendErr != nil {
				return teardown()
			}
		case <-stream.Context().Done():
			return teardown()
		}
	}
}
