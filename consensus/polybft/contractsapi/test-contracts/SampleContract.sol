// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

contract SampleContract {
    address public seller;
    uint256 public value;
    address public buyer;
    uint8 public state;

    event Aborted();
    event PurchaseConfirmed();
    event ItemReceived();
    event Refunded();

    constructor() public {
        seller = address(0);
        value = 0;
        buyer = address(0);
        state = 0;
    }

    function abort() external {
        emit Aborted();
    }

    function confirmPurchase() external returns (bool) {
        emit PurchaseConfirmed();
        return true;
    }

    function confirmReceived() external returns (bool) {
        emit ItemReceived();
        return true;
    }

    function refund() external returns (bool) {
        emit Refunded();
        return true;
    }
}
