// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

contract TestBenchmarkC {
    uint256 public valC;
    function fnC1() external returns (uint256) {
        uint256 valC2 = fnC2();
        valC++;
        return valC2;
    }

		function fnC2() public view returns (uint256) {
		    return uint256(keccak256(abi.encode(block.timestamp, block.difficulty))) % 100;
		}
}