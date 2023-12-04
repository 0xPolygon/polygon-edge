## What is JSON-RPC used for?

JSON-RPC is a remote procedure call (RPC) protocol encoded in JSON. It enables the communication between distributed systems, often found in microservices architectures. JSON-RPC is a lightweight, language-agnostic protocol that allows developers to build scalable applications with low-latency communication between services. With its simplicity and support for multiple programming languages, JSON-RPC offers an accessible and flexible tool for building distributed systems.

## JSON-RPC in Edge

JSON-RPC is implemented in Edge as an API consensus.

When a client makes a remote procedure call to the server, the JSON-RPC protocol abstracts the details of network communication, serialization, and deserialization. The JSON-RPC client sends a request message to the JSON-RPC server, which deserializes the request message, executes the appropriate method, and serializes the response message. The JSON-RPC server then sends the response message back to the client, which deserializes the response message and returns the result to the caller.

!!! info "Breakdown of the JSON-RPC API"

    The JSON-RPC implementation in Edge consists of several key components, including the `JSONRPC` struct and the `JSONRPCStore` interface.

    The `JSONRPC` struct handles the core functionality of the JSON-RPC server. It includes methods for setting up the HTTP server, handling WebSocket connections, and managing incoming requests. The `NewJSONRPC()` function is used to create a new instance of the JSONRPC server with a specified logger and configuration.

    The JSONRPCStore interface defines all the methods required by the JSON-RPC endpoints. These methods are implemented by various store types, such as `ethStore`, `networkStore`, `txPoolStore`, `filterManagerStore`, `bridgeStore`, and `debugStore`. These store types interact with different aspects of the system, allowing the JSON-RPC server to provide a comprehensive API for clients.

    For handling WebSocket connections, a `handleWs` function is used to upgrade HTTP connections to WebSocket connections. A `wsWrapper` struct wraps WebSocket connections and provides methods for managing WebSocket communication.
