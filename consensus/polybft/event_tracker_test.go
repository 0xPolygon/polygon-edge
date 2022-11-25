package polybft

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/testutil"
)

type mockEventSubscriber struct {
	lock sync.RWMutex
	logs []*ethgo.Log
}

func (m *mockEventSubscriber) AddLog(log *ethgo.Log) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if len(m.logs) == 0 {
		m.logs = []*ethgo.Log{}
	}

	m.logs = append(m.logs, log)
}

func (m *mockEventSubscriber) getLogs() []*ethgo.Log {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.logs
}

func TestEventTracker_TrackSyncEvents(t *testing.T) {
	t.Parallel()

	server := testutil.NewTestServer(t, nil)
	defer server.Close()

	tmpDir, err := os.MkdirTemp("/tmp", "test-event-tracker")
	defer os.RemoveAll(tmpDir)
	require.NoError(t, err)

	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `
			event StateSync(uint256 indexed id, address indexed target, bytes data);

			function emitEvent() public payable {
				emit StateSync(1, msg.sender, bytes(""));
			}
			`
	})

	_, addr := server.DeployContract(cc)

	// prefill with 10 events
	for i := 0; i < 10; i++ {
		server.TxnTo(addr, "emitEvent")
	}

	sub := &mockEventSubscriber{}

	tracker := &eventTracker{
		logger:     hclog.NewNullLogger(),
		subscriber: sub,
		dataDir:    tmpDir,
		config: &PolyBFTConfig{
			Bridge: &BridgeConfig{
				JSONRPCEndpoint: server.HTTPAddr(),
				BridgeAddr:      types.Address(addr),
			},
		},
	}

	err = tracker.start()
	require.NoError(t, err)

	time.Sleep(2 * time.Second)
	require.Len(t, sub.getLogs(), 10)
	// send 10 more events
	for i := 0; i < 10; i++ {
		server.TxnTo(addr, "emitEvent")
	}

	time.Sleep(2 * time.Second)
	require.Len(t, sub.getLogs(), 20)
}
