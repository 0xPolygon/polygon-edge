To enable the debug route namespace, you need to modify the configuration and add the "debug" parameter as shown below:

```
[jsonrpc.http]
    enabled = true
    port = 8545
    host = "0.0.0.0"
    api = ["eth", "net", "web3", "txpool", "bor", "debug"]
```

## debug_traceBlockByNumber

Executes all transactions in the block specified by number with a tracer and returns the tracing result.

### Parameters

* <b>QUANTITY|TAG </b> - integer of a block number, or the string "latest"
* <b> Object </b> - The tracer options:

  +  <b>  enableMemory: Boolean </b> - (optional, default: false) The flag indicating enabling memory capture.
  +  <b>  disableStack: Boolean </b> - (optional, default: false) The flag indicating disabling stack capture.
  +  <b>  disableStorage: Boolean </b> - (optional, default: false) The flag indicating disabling storage capture.
  +  <b>  enableReturnData: Boolean </b> - (optional, default: false) The flag indicating enabling return data capture.
  +  <b>  timeOut: String </b> - (optional, default: "5s") The timeout for cancellation of execution.
  +  <b>  tracer: String </b> - (default: "structTracer") Defines the debug tracer used for given call. Supported values: structTracer, callTracer.


### Returns

<b> Array </b> - Array of trace objects with the following fields:

  * <b> failed: Boolean </b> - the tx is successful or not
  * <b> gas: QUANTITY </b> - the total consumed gas in the tx
  * <b> returnValue: DATA </b> - the return value of the executed contract call
  * <b> structLogs: Array </b> - the trace result of each step with the following fields:

    + <b> pc: QUANTITY </b> - the current index in bytecode
    + <b> op: String </b> - the name of current executing operation
    + <b> gas: QUANTITY </b> - the available gas ßin the execution
    + <b> gasCost: QUANTITY </b> - the gas cost of the operation
    + <b> depth: QUANTITY </b> - the number of levels of calling functions
    + <b> error: String </b> - the error of the execution
    + <b> stack: Array </b> - array of values in the current stack
    + <b> memory: Array </b> - array of values in the current memory
    + <b> storage: Object </b> - mapping of the current storage
    + <b> refund: QUANTITY </b> - the total of current refund value

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"debug_traceBlockByNumber","params":["latest"],"id":1}'
````

## debug_traceBlockByHash

Executes all transactions in the block specified by block hash with a tracer and returns the tracing result.

### Parameters

* <b> DATA , 32 Bytes </b> - Hash of a block.
* <b> Object </b> - The tracer options. See debug_traceBlockByNumber for more details.

### Returns

<b> Array </b> - Array of trace objects. See debug_traceBlockByNumber for more details.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"debug_traceBlockByHash","params":["0xdc0818cf78f21a8e70579cb46a43643f78291264dda342ae31049421c82d21ae"],"id":1}'
````

## debug_traceBlock

Executes all transactions in the block given from the first argument with a tracer and returns the tracing result.

### Parameters

* <b> DATA </b> - RLP Encoded block bytes
* <b> Object </b> - The tracer options. See debug_traceBlockByNumber for more details.

### Returns

<b> Array </b> - Array of trace objects. See debug_traceBlockByNumber for more details.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"debug_traceBlock","params":["0xf9...."],"id":1}'
````

## debug_traceTransaction

Executes the transaction specified by transaction hash with a tracer and returns the tracing result.

### Parameters

* <b> DATA , 32 Bytes </b> - Hash of a transaction.
* <b> Object </b> - The tracer options. See debug_traceBlockByNumber for more details.

### Returns

<b> Object </b> - Trace object. See debug_traceBlockByNumber for more details.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"debug_traceTransaction","params":["0xdc0818cf78f21a8e70579cb46a43643f78291264dda342ae31049421c82d21ae"],"id":1}'
````

## debug_traceCall

Executes a new message call with a tracer and returns the tracing result.

### Parameters

* <b> Object </b>  - The transaction call object

  +  <b>  from: DATA, 20 Bytes </b> - (optional) The address the transaction is sent from.
  +  <b>  to: DATA, 20 Bytes </b> - The address the transaction is directed to.
  +  <b>  gas: QUANTITY </b> - (optional) Integer of the gas provided for the transaction execution. eth_call consumes zero gas, but this parameter may be needed by some executions.
  +  <b>  gasPrice: QUANTITY </b> - (optional) Integer of the gasPrice used for each paid gas
  +  <b>  value: QUANTITY </b> - (optional) Integer of the value sent with this transaction
  +  <b>  data: DATA </b> - (optional) Hash of the method signature and encoded parameters. For details see Ethereum Contract ABI in the Solidity documentation

* <b> QUANTITY|TAG </b> - integer block number, or the string "latest"
* <b> Object </b> - The tracer options. See debug_traceBlockByNumber for more details.

### Returns

<b> Object </b> - Trace object. See debug_traceBlockByNumber for more details.

### Example

````bash
curl  https://rpc-endpoint.io:8545 -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"debug_traceCall","params":[{"to": "0x1234", "data": "0x1234"}, "latest", {}],"id":1}'
````
