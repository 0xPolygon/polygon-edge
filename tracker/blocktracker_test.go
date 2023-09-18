package tracker

import (
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/blocktracker"
	"github.com/umbracle/ethgo/testutil"
)

func TestBlockTracker_PopulateBlocks(t *testing.T) {
	t.Parallel()

	// more than maxBackLog blocks
	{
		l := testutil.MockList{}
		l.Create(0, 15, func(b *testutil.MockBlock) {})

		m := &testutil.MockClient{}
		m.AddScenario(l)

		tt0 := NewBlockTracker(m, hclog.NewNullLogger())

		require.NoError(t, tt0.Init())
		require.True(t, testutil.CompareBlocks(l.ToBlocks()[5:], tt0.blocks))
	}
	// less than maxBackLog
	{
		l0 := testutil.MockList{}
		l0.Create(0, 5, func(b *testutil.MockBlock) {})

		m1 := &testutil.MockClient{}
		m1.AddScenario(l0)

		tt1 := NewBlockTracker(m1, hclog.NewNullLogger())
		tt1.provider = m1

		require.NoError(t, tt1.Init())
		require.True(t, testutil.CompareBlocks(l0.ToBlocks(), tt1.blocks))
	}
}

func TestBlockTracker_Events(t *testing.T) {
	t.Parallel()

	type TestEvent struct {
		Added   testutil.MockList
		Removed testutil.MockList
	}

	type Reconcile struct {
		block *testutil.MockBlock
		event *TestEvent
	}

	cases := []struct {
		Name      string
		Scenario  testutil.MockList
		History   testutil.MockList
		Reconcile []Reconcile
		Expected  testutil.MockList
	}{
		{
			Name: "Empty history",
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x1),
					event: &TestEvent{
						Added: testutil.MockList{
							testutil.Mock(0x1),
						},
					},
				},
			},
			Expected: []*testutil.MockBlock{
				testutil.Mock(1),
			},
		},
		{
			Name: "Repeated header",
			History: []*testutil.MockBlock{
				testutil.Mock(0x1),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x1),
				},
			},
			Expected: []*testutil.MockBlock{
				testutil.Mock(0x1),
			},
		},
		{
			Name: "New head",
			History: testutil.MockList{
				testutil.Mock(0x1),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x2),
					event: &TestEvent{
						Added: testutil.MockList{
							testutil.Mock(0x2),
						},
					},
				},
			},
			Expected: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
			},
		},
		{
			Name: "Ignore block already on history",
			History: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x3),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x2),
				},
			},
			Expected: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x3),
			},
		},
		{
			Name: "Multi Roll back",
			History: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x3),
				testutil.Mock(0x4),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x30).Parent(0x2),
					event: &TestEvent{
						Added: testutil.MockList{
							testutil.Mock(0x30).Parent(0x2),
						},
						Removed: testutil.MockList{
							testutil.Mock(0x3),
							testutil.Mock(0x4),
						},
					},
				},
			},
			Expected: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x30).Parent(0x2),
			},
		},
		{
			Name: "Backfills missing blocks",
			Scenario: testutil.MockList{
				testutil.Mock(0x3),
				testutil.Mock(0x4),
			},
			History: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x5),
					event: &TestEvent{
						Added: testutil.MockList{
							testutil.Mock(0x3),
							testutil.Mock(0x4),
							testutil.Mock(0x5),
						},
					},
				},
			},
			Expected: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x3),
				testutil.Mock(0x4),
				testutil.Mock(0x5),
			},
		},
		{
			Name: "Rolls back and backfills",
			Scenario: testutil.MockList{
				testutil.Mock(0x30).Parent(0x2).Num(3),
				testutil.Mock(0x40).Parent(0x30).Num(4),
			},
			History: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x3),
				testutil.Mock(0x4),
			},
			Reconcile: []Reconcile{
				{
					block: testutil.Mock(0x50).Parent(0x40).Num(5),
					event: &TestEvent{
						Added: testutil.MockList{
							testutil.Mock(0x30).Parent(0x2).Num(3),
							testutil.Mock(0x40).Parent(0x30).Num(4),
							testutil.Mock(0x50).Parent(0x40).Num(5),
						},
						Removed: testutil.MockList{
							testutil.Mock(0x3),
							testutil.Mock(0x4),
						},
					},
				},
			},
			Expected: testutil.MockList{
				testutil.Mock(0x1),
				testutil.Mock(0x2),
				testutil.Mock(0x30).Parent(0x2).Num(3),
				testutil.Mock(0x40).Parent(0x30).Num(4),
				testutil.Mock(0x50).Parent(0x40).Num(5),
			},
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			// safe check for now, we ma need to restart the tracker and mock client for every reconcile scenario?
			require.Len(t, c.Reconcile, 1)

			m := &testutil.MockClient{}

			// add the full scenario with the logs
			m.AddScenario(c.Scenario)

			tt := NewBlockTracker(m, hclog.NewNullLogger())

			// build past block history
			for _, b := range c.History.ToBlocks() {
				require.NoError(t, tt.AddBlockLocked(b))
			}

			sub := tt.Subscribe()
			for _, b := range c.Reconcile {
				require.NoError(t, tt.HandleTrackedBlock(b.block.Block()))

				if b.event == nil {
					continue
				}

				var blockEvnt *blocktracker.BlockEvent
				select {
				case blockEvnt = <-sub:
				case <-time.After(1 * time.Second):
					t.Fatal("block event timeout")
				}

				// check blocks
				require.True(t, testutil.CompareBlocks(b.event.Added.ToBlocks(), blockEvnt.Added))
				require.True(t, testutil.CompareBlocks(b.event.Removed.ToBlocks(), blockEvnt.Removed))
			}

			require.True(t, testutil.CompareBlocks(tt.blocks, c.Expected.ToBlocks()))
		})
	}
}
