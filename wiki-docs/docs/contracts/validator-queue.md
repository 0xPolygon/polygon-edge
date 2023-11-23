## Overview

`ValidatorQueue` is a library that can be used to manage a queue of
updates to block validators. The queue is used to register new validators,
add or remove stake, delegate or undelegate stake.

The queue is processed and cleared at the end of each epoch.

## Functions

### insert

```js
function insert(
    ValidatorQueue storage self,
    address validator,
    int256 stake,
    int256 delegation
) internal {
    uint256 index = self.indices[validator];
    if (index == 0) {
        // insert into queue
        // use index starting with 1, 0 is empty by default for easier checking of pending balances
        index = self.queue.length + 1;
        self.indices[validator] = index;
        self.queue.push(QueuedValidator(validator, stake, delegation));
    } else {
        // update values
        QueuedValidator storage queuedValidator = self.queue[indexOf(self, validator)];
        queuedValidator.stake += stake;
        queuedValidator.delegation += delegation;
    }
}
```

This function is used to queue a validator's `data`.
It takes three arguments: the `ValidatorQueue` struct, the `address` of the validator and
the change to the validator's `stake` and `delegation`. If the validator is not already
in the queue, the validator is added to the queue by adding a new `QueuedValidator` to the
queue array and updating the mapping of validator addresses to their position in the queue.

If the validator is already in the queue, the function updates the values of the
validator's `stake` and `delegation` in the `QueuedValidator` struct.

### resetIndex

```js
function resetIndex(ValidatorQueue storage self, address validator) internal {
    self.indices[validator] = 0;
}
```

This function is used to delete `data` from a specific validator in the `queue`.
It takes two arguments: the `ValidatorQueue` struct and the `address` of the validator
to remove the `queue` data of.

The function updates the mapping of validator addresses to their position in the `queue`
by setting the index for the validator to 0, indicating that the validator is not in
the `queue`.

### reset

```js
function reset(ValidatorQueue storage self) internal {
    delete self.queue;
}
```

This function is used to reinitialize the validator `queue`.
It takes one argument: the `ValidatorQueue` struct.

The function deletes the `queue` array, effectively resetting the `queue`.

### get

```js
function get(ValidatorQueue storage self) internal view returns (QueuedValidator[] storage) {
    return self.queue;
}
```

This function is used to return the `queue`. It takes one argument: the `ValidatorQueue` struct.

### waiting

```js
function waiting(ValidatorQueue storage self, address validator) internal view returns (bool) {
    return self.indices[validator] != 0;
}
```

This function is used to check if a specific validator is in the `queue`.
It takes two arguments: the `ValidatorQueue` struct and the `address` of the
validator to check.

### pendingState

```js
function pendingStake(ValidatorQueue storage self, address validator) internal view returns (int256) {
    return self.queue[self.indices[validator]].stake;
}
```

This function is used to return the change of `stake` for a validator in the `queue`.
It takes two arguments: the `ValidatorQueue` struct and the `address` of the validator
to check the change to `stake` of.

### pendingDelegation

```js
function pendingDelegation(ValidatorQueue storage self, address validator) internal view returns (int256) {
    return self.queue[self.indices[validator]].delegation;
}
```

This function is used to return the change to `delegation` for a validator
in the `queue`. It takes two arguments: the `ValidatorQueue` struct and
the `address` of the validator to check the change to delegation of.
