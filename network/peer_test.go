package network

/*
func testPeers(t *testing.T) (*Peer, *Peer) {
	c0, c1 := rlpx.TestP2PHandshake(t)

	info0 := rlpx.DummyInfo("info0", c0.LocalID)
	info1 := rlpx.DummyInfo("info1", c1.LocalID)

	if err := DoProtocolHandshake(c0, info0, c1, info1); err != nil {
		t.Fatal(err)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	return newPeer(logger, c0, info0, nil), newPeer(logger, c1, info1, nil)
}
*/
