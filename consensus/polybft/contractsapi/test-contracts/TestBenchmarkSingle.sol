// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

contract TestBenchmarkSingle {
	uint256[] private val;

    function addValue(uint256 value) public {
        val.push(value);
    }

    function getValue() public view returns (uint256[] memory) {
        return val;
    }

	    function compute(uint256 x, uint256 y) public pure returns (uint256) {
	        uint256 result = x + y;
	        for (uint256 i = 0; i < 10; i++) {
	            result = result * 2;
	        }
	        return result;
	    }
	}