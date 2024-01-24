// SPDX-License-Identifier: MIT
// Wrapper.sol
// Wrapper deploys (instantiates) NumberPersister
pragma solidity ^0.8.0;

import "./NumberPersister.sol";

contract Wrapper {
    NumberPersister public immutable _numberPersister = new NumberPersister();

    // A function to interact with NumberPersister and set its value
    function setNumber(uint256 _value) public {
        // Call setValue function in NumberPersister
        _numberPersister.setValue(_value);
    }

    // A function to get the value from NumberPersister
    function getNumber() public view returns (uint256) {
        // Read value from NumberPersister
        return _numberPersister.value();
    }
    
    function getAddress() public view returns (address){
        return address(_numberPersister);
    }
}
