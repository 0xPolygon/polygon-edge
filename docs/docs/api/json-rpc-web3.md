## web3_clientVersion

Returns the current client version.

### Parameters

None

### Returns


*  <b>  String </b> - The current client version

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"web3_clientVersion","params":[],"id":1}'
````

## web3_sha3

Returns Keccak-256 (not the standardized SHA3-256) of the given data.

### Parameters

*  <b> DATA </b> - the data to convert into a SHA3 hash

### Returns


*  <b>DATA </b> - The SHA3 result of the given string.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"web3_sha3","params":["0x68656c6c6f20776f726c64"],"id":1}'
````
