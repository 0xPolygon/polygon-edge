// SPDX-License-Identifier: MIT
// NumberPersister.sol
// Simple contract with a state variable
pragma solidity ^0.8.0;

contract NumberPersister {
    uint256 public value;

    // A function to set the value
    function setValue(uint256 _value) public {
        value = _value;
    }
}
