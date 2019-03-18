package minimal

// monitorMining is used to monitor when the chain is ready
// to mine new blocks
func (m *Minimal) monitorMining() {
	for {
		select {
		case synced := <-m.sealingCh:
			switch {
			case synced:
				// it is synced and ready to mine

			default:
				// it is not syncronized anymore

			}
		}
	}
}

// NOTE, maybe abstract this into another function
// inside minimal and just call for the sealer to start
// sealing if the chain is synced.
