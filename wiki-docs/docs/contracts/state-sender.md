## Overview

The `StateSender` contract is a simple smart contract that sends messages
to a child. Messages are indexed by validators from root and then signed.
Once they have enough signatures, they can be committed on `StateReceiver`
on child.

Unlike the current implementation of child, sending and receiving messages
is split into two contracts on root, where this one sends the state.

## Functions

### stateSync

```js
function syncState(address receiver, bytes calldata data) external {
        // check receiver
        require(receiver != address(0), "INVALID_RECEIVER");
        // check data length
        require(data.length <= MAX_LENGTH, "EXCEEDS_MAX_LENGTH");

        // State sync id will start with 1
        emit StateSynced(++counter, msg.sender, receiver, data);
    }
```

This function allows anyone to emit a `StateSynced` event by calling the function
and passing in a receiver `address` and `data`. The function has two require statements:
one that checks that the receiver address is not the zero address, and one that checks
that the length of the data is less than or equal to the `MAX_LENGTH`, which is **2048**.
If either of these checks fail, the contract will revert with an appropriate error
message.

If the checks pass, the function will emit a `StateSynced` event with the `id`, `sender`,
`receiver`, and `data`.
