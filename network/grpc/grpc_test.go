package grpc

/*
type testService struct {
	test.UnimplementedTestServer
}

func (t *testService) A(ctx context.Context, req *test.AReq) (*test.AResp, error) {
	fmt.Println("- a -")
	return &test.AResp{}, nil
}

func TestGrpcStream(t *testing.T) {
	port := 2000
	streamID := "some-id"

	createServer := func() (*network.Server, *testService) {
		// create the server
		srv, err := network.NewServer("", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
		assert.NoError(t, err)
		port++

		// create the grpc protocol
		service := &testService{}
		stream := NewGrpcStream()

		test.RegisterTestServer(stream.GrpcServer(), service)

		// register the grpc protocol as a stream
		srv.Register(streamID, stream)

		return srv, service
	}

	srv0, service0 := createServer()
	srv1, service1 := createServer()

	fmt.Println(srv0, service0)
	fmt.Println(srv1, service1)

	// connect with 0 -> 1
	peer, err := srv0.Connect(srv1.AddrInfo())
	assert.NoError(t, err)

	// open a grpc connection
	fmt.Println(peer)

	ss := srv0.CC(srv1.AddrInfo().ID, streamID)

	conn := &streamConn{ss}
	clt := test.NewTestClient(conn.grpcClient())
	clt.A(context.Background(), &test.AReq{})
}
*/
