package polybft

import (
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/testutil"
)

type mockEventSubscriber struct {
	logs []*ethgo.Log
}

func (m *mockEventSubscriber) AddLog(log *ethgo.Log) {
	if len(m.logs) == 0 {
		m.logs = []*ethgo.Log{}
	}

	m.logs = append(m.logs, log)
}

func TestEventTracker_TrackSyncEvents(t *testing.T) {
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
		logger:     newTestLogger(),
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
	require.Len(t, sub.logs, 10)

	// send 10 more events
	for i := 0; i < 10; i++ {
		server.TxnTo(addr, "emitEvent")
	}

	time.Sleep(2 * time.Second)
	require.Len(t, sub.logs, 20)
}
