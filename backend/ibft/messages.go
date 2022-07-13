package ibft

import (
	"github.com/Trapesys/go-ibft/messages/proto"
)

func (i *Ibft) BuildPrePrepareMessage(proposal []byte, view *proto.View) *proto.Message {

	return nil
}

func (i *Ibft) BuildPrepareMessage(proposal []byte, view *proto.View) *proto.Message {

	return nil
}

func (i *Ibft) BuildCommitMessage(proposal []byte, view *proto.View) *proto.Message {
	return nil
}

func (i *Ibft) BuildRoundChangeMessage(height, round uint64) *proto.Message {
	return nil
}
