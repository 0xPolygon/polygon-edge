// SPDX-License-Identifier: MIT
pragma solidity 0.8.19;

interface ITestBenchmarkC {
    function fnC1() external returns (uint256);
}
contract TestBenchmarkB {
    uint256 public valB;
    address contractAddr;

    function setContractAddr(address _contract) public {
       contractAddr = _contract;
    }

    function fnB() external returns (uint256) {
        uint256 valC = ITestBenchmarkC(contractAddr).fnC1();
        valB += valC;
        return valC;
    }
}