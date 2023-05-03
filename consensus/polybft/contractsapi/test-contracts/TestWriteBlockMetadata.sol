// SPDX-License-Identifier: MIT
pragma solidity 0.8.17;

contract TestWriteBlockMetadata {
    uint[] public data;
    address public coinbase;
    // bytes32 public hash;

    function init() external {
        data.push(block.difficulty);
        data.push(block.number);
        data.push(block.timestamp);        
        data.push(block.gaslimit);
        data.push(block.chainid);
        coinbase = block.coinbase;
        data.push(block.basefee);
        // hash = blockhash(block.number);
    }
}