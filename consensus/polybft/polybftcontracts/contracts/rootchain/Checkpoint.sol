//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

import "./RootchainBridge.sol";

contract Checkpoint {
    function checkpoint(uint256 _checkpoint) public {
        // 1. TODO: data checkpoint

        // 2. emit bridge checkpoint event
        RootchainBridge c = RootchainBridge(0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27);
        c.emitEvent(0xBd770416a3345F91E4B34576cb804a576fa48EB1, abi.encode(_checkpoint));
    }
}
