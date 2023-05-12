# Benchmark tests
The benchmark folder contains benchmark tests for the smart contracts.

## Common directory
In the common directory, you'll find:
- the contract_code.go file that contains byte codes for the contracts used in the tests,
- helpers.go that has helper functions needed in tests like deploying contracts and similar,
- executors.go that holds executors which execute the test cases. For example, there is an executor that submits multiple transactions in parallel and measures the execution time.

## Tests
All test scenarios are executed in benchmark_test.go. Decoupling test scenarios from execution enables the usage of different scenarios in the benchmark command, which is written for the purpose of executing the benchmark test on a test environment. The command can be run with in a following way:
polygon-edge benchmark-test --childJSONRPC="http://127.0.0.1:12001" --rootJSONRPC="http://127.0.0.1:8545" --privateKey="aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"

## Testing tx send on root and child chains
The RootChildSendTx function executes test cases that measure transaction execution on both the root and child chains. To do this, it first calls RootChildSendTxSetUp to set up the testing environment, which may include starting the cluster, deploying contracts, and building the test cases. After building the test cases, RootChildSendTx returns them along with a cleanup function that should be called after the test cases have been executed.

The test cases are executed by the TxTestCasesExecutor. The RootJSONRPC, ChildJSONRPC, and PrivateKey flags are used to configure the testing environment. If all of these flags are set, then the local cluster will not be started and the provided addresses will be used as the endpoints to the root and child chains. If any of these flags is not set, the local cluster will be started automatically. If the private key is specified, it will be used as the transaction sender. Otherwise, the local cluster will generate a sender key. If the cluster is not run locally, then the sender must have enough funds for sending transactions.