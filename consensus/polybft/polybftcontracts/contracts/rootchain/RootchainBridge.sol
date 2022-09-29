//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

contract RootchainBridge {
    event StateSync(
        uint256 indexed id,
        address indexed sender,
        address indexed target,
        bytes data
    );
    uint64 index;

    function emitEvent(address target, bytes memory data) public {
        emit StateSync(index, msg.sender, target, data);
        index++;
    }
}
