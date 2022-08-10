package monitor

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
)

const (
	eventAdded   = "ADD BLOCK"
	eventRemoved = "REMOVE BLOCK"
)

type BlockchainEvent struct {
	Type   string `json:"type"`
	Number int64  `json:"number"`
	Hash   string `json:"hash"`
}

type BlockChainEvents struct {
	Added   []BlockchainEvent `json:"added"`
	Removed []BlockchainEvent `json:"removed"`
}

type BlockEventResult struct {
	Events BlockChainEvents `json:"events"`
}

func NewBlockEventResult(e *proto.BlockchainEvent) *BlockEventResult {
	res := &BlockEventResult{
		Events: BlockChainEvents{
			Added:   make([]BlockchainEvent, len(e.Added)),
			Removed: make([]BlockchainEvent, len(e.Removed)),
		},
	}

	for i, add := range e.Added {
		res.Events.Added[i].Type = eventAdded
		res.Events.Added[i].Number = add.Number
		res.Events.Added[i].Hash = add.Hash
	}

	for i, rem := range e.Removed {
		res.Events.Removed[i].Type = eventRemoved
		res.Events.Removed[i].Number = rem.Number
		res.Events.Removed[i].Hash = rem.Hash
	}

	return res
}

func (r *BlockEventResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[BLOCK EVENT]\n")

	for _, event := range r.getCombinedEvents() {
		buffer.WriteString(helper.FormatKV([]string{
			fmt.Sprintf("Event Type|%s", event.Type),
			fmt.Sprintf("Block Number|%d", event.Number),
			fmt.Sprintf("Block Hash|%s", event.Hash),
		}))
	}

	return buffer.String()
}

func (r *BlockEventResult) getCombinedEvents() []BlockchainEvent {
	events := append([]BlockchainEvent{}, r.Events.Added...)

	return append(events, r.Events.Removed...)
}
