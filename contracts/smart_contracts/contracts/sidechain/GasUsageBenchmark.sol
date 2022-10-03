// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "./SidechainBridge.sol";

contract GasUsageBenchmark is BridgeImpl {
    uint64 counter;

    function onStateReceive(
        uint64, /*index*/
        address, /*sender*/
        bytes memory data
    ) external payable override {
        uint64 iterations = abi.decode(data, (uint64));
        for (uint64 i = 0; i < iterations; i++) {
            counter++;
        }
    }
}
