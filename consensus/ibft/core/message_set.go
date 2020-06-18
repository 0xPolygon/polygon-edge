package core

import (
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
)

// Construct a new message set to accumulate messages for given sequence/view number.
func newMessageSet(valSet ibft.ValidatorSet) *messageSet {
	return &messageSet{
		view: &ibft.View{
			Round:    new(big.Int),
			Sequence: new(big.Int),
		},
		messagesMu: new(sync.Mutex),
		messages:   make(map[types.Address]*message),
		valSet:     valSet,
	}
}

// ----------------------------------------------------------------------------

type messageSet struct {
	view       *ibft.View
	valSet     ibft.ValidatorSet
	messagesMu *sync.Mutex
	messages   map[types.Address]*message
}

func (ms *messageSet) View() *ibft.View {
	return ms.view
}

func (ms *messageSet) Add(msg *message) error {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	if err := ms.verify(msg); err != nil {
		return err
	}

	return ms.addVerifiedMessage(msg)
}

func (ms *messageSet) Values() (result []*message) {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	for _, v := range ms.messages {
		result = append(result, v)
	}

	return result
}

func (ms *messageSet) Size() int {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return len(ms.messages)
}

func (ms *messageSet) Get(addr types.Address) *message {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return ms.messages[addr]
}

// ----------------------------------------------------------------------------

func (ms *messageSet) verify(msg *message) error {
	// verify if the message comes from one of the validators
	if _, v := ms.valSet.GetByAddress(msg.Address); v == nil {
		return ibft.ErrUnauthorizedAddress
	}

	// TODO: check view number and sequence number

	return nil
}

func (ms *messageSet) addVerifiedMessage(msg *message) error {
	ms.messages[msg.Address] = msg
	return nil
}

func (ms *messageSet) String() string {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	addresses := make([]string, 0, len(ms.messages))
	for _, v := range ms.messages {
		addresses = append(addresses, v.Address.String())
	}
	return fmt.Sprintf("[%v]", strings.Join(addresses, ", "))
}
