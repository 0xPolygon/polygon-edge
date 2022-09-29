// contracts/GLDToken.sol
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

// Implementation of the erc20 token with a mint function. This contract uses
// the mint function because it has to 'generate' the new tokens in the chain
// whenever they are created on the other side.
contract MintERC20 is ERC20 {
    constructor() ERC20("Gold", "GLD") {}

    event Mint(address to, uint256 amount);

    function mint(uint256 amount) public payable {
        _mint(msg.sender, amount);
    }

    function mintTo(address to, uint256 amount) public payable {
        _mint(to, amount);
        emit Mint(to, amount);
    }
}
