package backend

import (
	"errors"
	"io"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

// TODO: Remove
// This is done in libp2p

const (
	istanbulMsg = 0x11
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
)

/*
// Protocol implements consensus.Engine.Protocol
func (sb *backend) Protocol() consensus.Protocol {
	return consensus.Protocol{
		Name:     "istanbul",
		Versions: []uint{64},
		Lengths:  []uint64{18},
	}
}
*/

type Msg struct {
	Code    uint64
	Payload io.Reader
	Size    uint64
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *backend) HandleMsg(addr types.Address, msg Msg) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	if msg.Code == istanbulMsg {
		if !sb.coreStarted {
			return true, ibft.ErrStoppedEngine
		}

		var data []byte

		s := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if err := s.Decode(&data); err != nil {
			return false, err
		}

		hash := ibft.RLPHash(data)

		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		// Mark self known message
		if _, ok := sb.knownMessages.Get(hash); ok {
			return true, nil
		}
		sb.knownMessages.Add(hash, true)

		go sb.istanbulEventMux.Post(ibft.MessageEvent{
			Payload: data,
		})

		return true, nil
	}
	return false, nil
}

/*
// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}
*/

func (sb *backend) NewChainHead() error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return ibft.ErrStoppedEngine
	}
	go sb.istanbulEventMux.Post(ibft.FinalCommittedEvent{})
	return nil
}
