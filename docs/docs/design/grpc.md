## What is gRPC used for?

gRPC is a popular RPC (remote procedure call) framework that enables efficient communication between distributed systems. It is commonly used in microservices architectures and is especially useful for building high-performance, scalable applications that require low-latency communication between services. With its support for multiple programming languages and its built-in support for data serialization, gRPC provides a powerful and flexible tool for building distributed systems.

## gRPC in Edge

libp2p provides the underlying networking layer for establishing connections between peers. By creating a gRPC server instance on each peer in the network and allowing gRPC client connections to be established, a network API can be created that enables clients to interact with the network.

When a client makes a remote procedure call to the server, the gRPC protocol abstracts the details of network communication, serialization, and deserialization. The gRPC client sends a request message to the gRPC server, which deserializes the request message, executes the appropriate method, and serializes the response message. The gRPC server then sends the response message back to the client, which deserializes the response message and returns the result to the caller.

!!! info "Breakdown of the gRPC API"

    The `GrpcStream` struct lies at the core of the gRPC implementation, providing developers with a flexible and powerful tool for building decentralized applications on the network. By abstracting away the intricacies of network communication, it offers a simple and high-level API for constructing distributed systems.

    To create a new `GrpcStream` object and initialize a gRPC server instance, developers can call the `NewGrpcStream()` function. The `Client()` method allows developers to wrap a network stream in a gRPC client connection, and the `Serve()` method starts the gRPC server in a separate goroutine.

    For registering a gRPC service and its implementation with the server, developers can use the `RegisterService()` method. The `GrpcServer()` method returns the underlying gRPC server instance, making it simple to configure and manage.

    For greater flexibility and control over the network communication process, developers can use the `Accept()` method to wait for and accept incoming connections. The `WrapClient()` function can also wrap a network stream in a gRPC client connection.

