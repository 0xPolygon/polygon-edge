package ibft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/stretchr/testify/assert"
)

func mockQueueMsg(id string, state MsgType, view *proto.View) *msgTask {
	return &msgTask{
		view: view,
		msg:  state,
		obj: &proto.MessageReq{
			// use the from field to identify the msg
			From: id,
		},
	}
}

func TestMsgQueue_RoundChangeState(t *testing.T) {
	m := newMsgQueue()

	// insert non round change messages
	{
		m.pushMessage(mockQueueMsg("A", msgPrepare, proto.ViewMsg(1, 0)))
		m.pushMessage(mockQueueMsg("B", msgCommit, proto.ViewMsg(1, 0)))

		// we only read round state messages
		assert.Nil(t, m.readMessage(RoundChangeState, proto.ViewMsg(1, 0)))
	}

	// insert old round change messages
	{
		m.pushMessage(mockQueueMsg("C", msgRoundChange, proto.ViewMsg(1, 1)))

		// the round change message is old
		assert.Nil(t, m.readMessage(RoundChangeState, proto.ViewMsg(2, 0)))
		assert.Zero(t, m.roundChangeStateQueue.Len())
	}

	// insert two valid round change messages with an old one in the middle
	{
		m.pushMessage(mockQueueMsg("D", msgRoundChange, proto.ViewMsg(2, 2)))
		m.pushMessage(mockQueueMsg("E", msgRoundChange, proto.ViewMsg(1, 1)))
		m.pushMessage(mockQueueMsg("F", msgRoundChange, proto.ViewMsg(2, 1)))

		msg1 := m.readMessage(RoundChangeState, proto.ViewMsg(2, 0))
		assert.NotNil(t, msg1)
		assert.Equal(t, msg1.obj.From, "F")

		msg2 := m.readMessage(RoundChangeState, proto.ViewMsg(2, 0))
		assert.NotNil(t, msg2)
		assert.Equal(t, msg2.obj.From, "D")
	}
}

func TestCmpView(t *testing.T) {
	var cases = []struct {
		v, y *proto.View
		res  int
	}{
		{
			&proto.View{
				Sequence: 1,
				Round:    1,
			},
			&proto.View{
				Sequence: 2,
				Round:    1,
			},
			-1,
		},
	}

	for _, c := range cases {
		assert.Equal(t, cmpView(c.v, c.y), c.res)
	}
}
