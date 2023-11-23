## net_version

Returns the current network id.

### Parameters

None

### Returns


* <b> String </b> - The current network id.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_version","params":[],"id":83}'
````

## net_listening

Returns true if a client is actively listening for network connections.

### Parameters

None

### Returns


* <b> Boolean </b> - true when listening, otherwise false.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_listening","params":[],"id":83}'
````

## net_peerCount

Returns number of peers currently connected to the client.

### Parameters

None

### Returns


* <b> QUANTITY </b> - integer of the number of connected peers.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_peerCount","params":[],"id":1}'
````
