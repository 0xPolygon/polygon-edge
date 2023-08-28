package txpool

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// Status implements the GRPC status endpoint. Returns the number of transactions in the pool
func (p *TxPool) Status(ctx context.Context, req *empty.Empty) (*proto.TxnPoolStatusResp, error) {
	resp := &proto.TxnPoolStatusResp{
		Length: p.accounts.promoted(),
	}

	return resp, nil
}

// AddTxn adds a local transaction to the pool
func (p *TxPool) AddTxn(ctx context.Context, raw *proto.AddTxnReq) (*proto.AddTxnResp, error) {
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

	if err := p.AddTx(txn); err != nil {
		return nil, err
	}

	return &proto.AddTxnResp{
		TxHash: txn.Hash.String(),
	}, nil
}

// Subscribe implements the operator endpoint. It subscribes to new events in the tx pool
func (p *TxPool) Subscribe(
	request *proto.SubscribeRequest,
	stream proto.TxnPoolOperator_SubscribeServer,
) error {
	if err := request.ValidateAll(); err != nil {
		return err
	}

	subscription := p.eventManager.subscribe(request.Types)

	cancel := func() {
		p.eventManager.cancelSubscription(subscription.subscriptionID)
	}

	for {
		select {
		case event, more := <-subscription.subscriptionChannel:
			if !more {
				// Subscription is closed from some other place
				return nil
			}

			if sendErr := stream.Send(event); sendErr != nil {
				cancel()

				return nil
			}
		case <-stream.Context().Done():
			cancel()

			return nil
		}
	}
}

// TxPoolSubscribe subscribes to new events in the tx pool and returns subscription channel and unsubscribe fn
func (p *TxPool) TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error) {
	if err := request.ValidateAll(); err != nil {
		return nil, nil, err
	}

	subscription := p.eventManager.subscribe(request.Types)

	cancelSubscription := func() {
		p.eventManager.cancelSubscription(subscription.subscriptionID)
	}

	return subscription.subscriptionChannel, cancelSubscription, nil
}
