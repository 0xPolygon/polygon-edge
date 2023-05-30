// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

contract BigDataContract {
    bytes public data;
    event Written();

    function writeData(bytes memory newData) external returns (bytes memory) {
       data = abi.encodePacked(data, newData);
        emit Written();
        return data;
    }
}
