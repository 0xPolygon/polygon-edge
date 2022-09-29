//SPDX-License-Identifier: Unlicense
pragma solidity ^0.8.0;

interface BridgeImpl {
    function onStateReceive(
        uint64 _index,
        address sender,
        bytes memory data
    ) external payable;
}

// Bridge is the proxy that calls the specific bridge contracts
contract SidechainBridge {
    uint256 public constant MAX_GAS = 100000;
    // Index of the next state sync which is going to be committed.
    uint64 nextCommittedStateSyncIndex;
    // Index of the next state sync which is going to be executed.
    uint64 nextExecutionStateSyncIndex;

    // 0=success, 1=failure
    enum ResultStatus {
        SUCCESS,
        FAILURE
    }

    event ResultEvent(
        uint64 indexed index,
        address indexed sender,
        ResultStatus indexed status,
        bytes32 message
    );

    struct Signature {
        bytes aggregatedSignature;
        bytes bitmap;
    }

    struct Proof {
        bytes32 hash;
        bytes value;
    }

    struct StateSync {
        uint64 id;
        address sender;
        address target;
        bytes data;
    }

    struct Commitment {
        bytes32 merkleRoot;
        uint64 fromIndex;
        uint64 toIndex;
        uint64 bundleSize;
        uint64 epoch;
    }

    function registerCommitment(
        Commitment calldata commitment,
        Signature calldata /*signature*/
    ) external payable {
        // Omitted signature verification and caching commitments
        nextCommittedStateSyncIndex = commitment.toIndex + 1;
    }

    function executeBundle(
        Proof[] calldata, /*proof*/
        StateSync[] calldata stateSyncEvents
    ) public payable {
        require(
            stateSyncEvents.length > 0,
            "Provided state sync events bundle is empty"
        );
        // Omitted proof verification
        for (uint256 i = 0; i < stateSyncEvents.length; i++) {
            _executeStateSync(stateSyncEvents[i]);
        }
    }

    function _executeStateSync(StateSync memory _stateSync) private {
        // validate order
        require(
            nextExecutionStateSyncIndex == _stateSync.id,
            "Index must be sequential"
        );
        nextExecutionStateSyncIndex++;

        bytes memory paramData = abi.encodeWithSignature(
            "onStateReceive(uint64,address,bytes)",
            _stateSync.id,
            _stateSync.sender,
            _stateSync.data
        );

        (bool success, bytes memory returnData) = _stateSync.target.call{
            gas: MAX_GAS
        }(paramData);

        bytes32 message;

        assembly {
            message := mload(add(returnData, 32))
        }
        // emit a ResultEvent indicating whether invocation of bridge was successful or not
        if (success) {
            emit ResultEvent(
                _stateSync.id,
                _stateSync.sender,
                ResultStatus.SUCCESS,
                message
            );
        } else {
            emit ResultEvent(
                _stateSync.id,
                _stateSync.sender,
                ResultStatus.FAILURE,
                message
            );
        }
    }

    function getNextCommittedIndex() public view returns (uint64) {
        return nextCommittedStateSyncIndex;
    }

    function getNextExecutionIndex() public view returns (uint64) {
        return nextExecutionStateSyncIndex;
    }
}
