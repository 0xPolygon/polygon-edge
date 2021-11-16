pragma solidity ^0.8.0;

contract StressTest {
    string private name;
    uint256 private num;

    constructor (){
        num = 0;
    }

    event txnDone(uint number);

    function setName(string memory sName) external {
        num++;
        name = sName;
        emit txnDone(num);
    }

    function getCount() view external returns (uint){
        return num;
    }
}
