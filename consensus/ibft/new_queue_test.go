package ibft

import (
	"container/heap"
	"testing"
)

func TestQueuePriority(t *testing.T) {
	type mockMsg struct {
		raw      string
		sequence uint64
		round    uint64
		msg      uint64
	}

	cases := []struct {
		msgs []mockMsg
		res  []string
	}{
		{
			[]mockMsg{
				{"A", 1, 2, 3},
				{"B", 1, 1, 3},
			},
			[]string{"B", "A"},
		},
	}

	for _, i := range cases {
		q := queueImpl{}
		for _, msg := range i.msgs {
			task := &queueTask{
				sequence: msg.sequence,
				round:    msg.round,
				msg:      msg.msg,
				obj:      msg.raw,
			}
			heap.Push(&q, task)
		}
		if len(i.res) != q.Len() {
			t.Fatal("bad length")
		}
		for indx, item := range i.res {
			if item != q[indx].obj.(string) {
				t.Fatal("bad")
			}
		}
	}
}
