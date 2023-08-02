// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

contract TestL1StateReceiver {
    uint256 public id;
    address public addr;
    bytes public data;
    uint256 public counter;

    function onL2StateReceive(
        uint256 _id,
        address _addr,
        bytes memory _data
    ) public {
        id = _id;
        addr = _addr;
        data = _data;
        counter++;
    }
}
