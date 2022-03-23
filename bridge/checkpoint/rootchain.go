package checkpoint

type RootChainContractClient interface {
	GetLastChildBlock() (uint64, error)
	// GetCurrentHeaderBlock() (uint64, error)
	// SubmitCheckpoint(data *ctypes.Checkpoint, signatures [][]byte) (types.Hash, error)
}
