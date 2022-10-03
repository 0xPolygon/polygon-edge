// contracts/GLDToken.sol
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "./SidechainBridge.sol";

interface ERC20BridgeToken {
    function mintTo(address to, uint256 amount) external payable;
}

contract ERC20Bridge is BridgeImpl {
    ERC20BridgeToken target;

    constructor(ERC20BridgeToken _target) {
        target = _target;
    }

    // TODO: How to utilize index and sender parameters?
    function onStateReceive(
        uint64, /*index*/
        address, /*sender*/
        bytes memory data
    ) external payable override {
        (address to, uint256 amount) = abi.decode(data, (address, uint256));
        target.mintTo(to, amount);
    }
}
