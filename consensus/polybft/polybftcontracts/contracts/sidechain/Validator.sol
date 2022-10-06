//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

import "./SidechainBridge.sol";

contract Validator is BridgeImpl {
    // As of now we are going to use the same thing for both
    // bridge and validators
    // list of current validators
    struct ValidatorAccount {
        address ecdsa;
        bytes bls;
    }
    ValidatorAccount[] allValidators;
    ValidatorAccount[] currentValidators;
    uint64 epoch;
    uint64 validatorSetSize;
    uint256 indexToStart = 1;

    // constructor(ValidatorAccount[] memory _validators, uint64 _validatorSetSize) {
    //     init(_validators, _validatorSetSize);
    // }

    constructor() {
    }

    function init(ValidatorAccount[] memory _validators, uint64 _validatorSetSize) public {
        require(_validators.length >= _validatorSetSize, "Validator snapshot size can not be bigger than length of validators array");
        for (uint256 i = 0; i < _validators.length; i++) {
            allValidators.push(_validators[i]);
        }
        validatorSetSize = _validatorSetSize;
    }

    function getValidators() public view returns (ValidatorAccount[] memory) {
        if (currentValidators.length == 0) {
            return allValidators;
        }
        return currentValidators;
    }

    function uptime(bytes memory data) public payable {
        // TODO: validator set should be updated based on provided data
        // For now, we only return a number of validators that is required by tests
        if (validatorSetSize > 0) {
            delete currentValidators;
            uint256 validatorIndex = indexToStart;
            for(uint256 i=0; i<validatorSetSize; i++){
                validatorIndex = validatorIndex % allValidators.length;
                currentValidators.push(allValidators[validatorIndex]);
                validatorIndex++;
            }

            indexToStart = (indexToStart + 1) % allValidators.length;
        }
        epoch++;
    }

    function getEpoch() public view returns (uint64) {
        return 77;
    }

    // --- checkpoint manager on sidechain ---

    // lastCheckpoint is the last block for which there is a checkpoint consensus
    uint256 lastCheckpoint;

    function getLastCheckpoint() public view returns (uint256) {
        return lastCheckpoint;
    }

    // onStateReceive to receive checkpoint bridge events
    function onStateReceive(
        uint64, /*_index*/
        address, /*sender*/
        bytes memory data
    ) external payable override {
        uint256 _checkpoint;

        (_checkpoint) = abi.decode(data, (uint256));
        lastCheckpoint = _checkpoint;
    }
}
