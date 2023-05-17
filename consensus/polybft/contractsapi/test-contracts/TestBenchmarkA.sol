// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

interface ITestBenchmarkB {
    function fnB() external returns (uint256);
}
contract TestBenchmarkA {
    address contractAddr;

    function setContractAddr(address _contract) public {
       contractAddr = _contract;
    }

	    function fnA() public returns (uint256) {
	        uint256 valB = ITestBenchmarkB(contractAddr).fnB();
	        return valB;
	    }
	}