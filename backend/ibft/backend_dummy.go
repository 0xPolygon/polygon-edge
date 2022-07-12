package ibft

//	backend impl for go-ibft

func (i *Ibft) BuildProposal(blockNumber uint64) ([]byte, error) {
	return nil, nil
}

func (i *Ibft) InsertBlock(proposal []byte, committedSeals [][]byte) error {
	return nil
}

func (i *Ibft) ID() []byte {
	return nil
}

func (i *Ibft) MaximumFaultyNodes() uint64 {
	return 0
}

func (i *Ibft) Quorum(blockNumber uint64) uint64 {
	return 0
}
