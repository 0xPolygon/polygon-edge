## Overview

The `RootValidatorSet` contract is an upgradeable contract that is
used to register validators and their ECDSA and BLS public keys on
Polygon.

:::caution Current validator set

To bootstrap, the `RootValidatorSet` is composed of internal validators
of Polygon Technology.

:::

## Functions

### initialize

```js
function initialize(
    address governance,
    address newCheckpointManager,
    address[] calldata validatorAddresses,
    uint256[4][] calldata validatorPubkeys
) external initializer {
    require(validatorAddresses.length == validatorPubkeys.length, "LENGTH_MISMATCH");
    // slither-disable-next-line missing-zero-check
    checkpointManager = newCheckpointManager;
    uint256 currentId = 0; // set counter to 0 assuming validatorId is currently at 0 which it should be...
    for (uint256 i = 0; i < validatorAddresses.length; i++) {
        Validator storage newValidator = validators[++currentId];
        newValidator._address = validatorAddresses[i];
        newValidator.blsKey = validatorPubkeys[i];

        validatorIdByAddress[validatorAddresses[i]] = currentId;

        emit NewValidator(currentId, validatorAddresses[i], validatorPubkeys[i]);
    }
    currentValidatorId = currentId;
    _transferOwnership(governance);
}
```

This function is the initialization function for the `RootValidatorSet` contract.
It is used to set up the initial state of the contract. The function takes in four arguments:

- `governance`: the address of the governance contract that will own
  the `RootValidatorSet` contract after it is initialized
- `newCheckpointManager`: the address of the `CheckpointManager` for the contract
- `validatorAddresses`: an array of validator addresses to seed the contract with
- `validatorPubkeys`: an array of validator pubkeys to seed the contract with

The function has a require statement that checks that the length of
`validatorAddresses` is equal to the length of the `validatorPubkeys`.
If these are not the same length, the function reverts with an error message.

The function then sets the value of the `checkpointManager` to the
`newCheckpointManager` argument. It then iterates over `validatorAddresses`
and registers each address as a validator. It does this by creating a
new `Validator` and storing it in the `validators` mapping using the next available
validator `id` as the `key`. The `validatorIdByAddress` mapping is also updated
to store the validator `id` corresponding to the validator `address`.

After all the validators have been registered, `currentValidatorId`
is updated to the highest validator `id` that was assigned.

Finally, contract ownership is transferred to the `governance` contract that
was passed in as an argument.

### addValidators

```js
function addValidators(Validator[] calldata newValidators) external {
    require(msg.sender == checkpointManager, "ONLY_CHECKPOINT_MANAGER");
    uint256 length = newValidators.length;
    uint256 currentId = currentValidatorId;

    for (uint256 i = 0; i < length; i++) {
        validators[i + currentId + 1] = newValidators[i];

        emit NewValidator(currentId + i + 1, newValidators[i]._address, newValidators[i].blsKey);
    }

    currentValidatorId += length;
}
```

This function allows the `checkpointManager` to add new validators to the contract.
It takes in an array of Validators and iterates over them, adding each one to
the contract and emitting a `NewValidator` event for each one.

The function also increments `currentValidatorId` by the
length of the array. The function has a require statement that checks that the caller
is the `checkpointManager`, and will revert with an error message if the caller is not.

### getValidators

```js
function getValidator(uint256 id) external view returns (Validator memory) {
    return validators[id];
}
```

This function allows anyone to retrieve the `Validator` struct for a
specific validator by `id`. It takes in an `id` as an argument and returns the
corresponding `Validator` struct from the `validators` mapping.

### getValidatorBlsKey

```js
function getValidatorBlsKey(uint256 id) external view returns (uint256[4] memory) {
    return validators[id].blsKey;
}
```

This function is a convenience function that allows anyone to retrieve the
BLS public key of a specific validator by `id`. It takes in an `id` as an
argument and returns the `blsKey` stored in the `Validator` struct for that
`id`.

### getValidatorId

```js
function getValidatorId(address address) external view returns (uint256) {
    return validatorIdByAddress[address];
}
```

This function allows anyone to retrieve the `id` of a validator by their `address`.
It takes in an `address` as an argument and returns the corresponding
`id` from the `validatorIdByAddress` mapping.

### getValidatorCount

```js
function getValidatorCount() external view returns (uint256) {
    return currentValid;
}
```

This function allows anyone to retrieve the total number of validators
that are registered in the contract. It returns the
value of `currentValidatorId`, which is the highest validator `id`
that has been assigned to a validator. The `currentValidatorId` is
incremented each time a new validator is added to the contract, so the number
returned by this function represents the total number of validators that have
been registered in the contract.

### isValidator

```js
function isValidator(address _address) external view returns (bool) {
    return validatorIdByAddress[_address] != 0;
}
```

This function allows anyone to check if a given address is registered
as a validator. It takes in an address as an argument and
indicates whether the address is registered as a validator.

The function checks the `validatorIdByAddress` mapping to see if the given
`address` has an `id` associated with it.

### isValidatorActive

```js
function isValidatorActive(uint256 _validatorId) external view returns (bool) {
    return _validatorId <= currentValidatorId && _validatorId > currentValidatorId - ACTIVE_VALIDATOR_SET_SIZE;
}
```

This function allows anyone to check if a given validator is active.
It takes in a validator `id` as an argument and indicates whether the validator
is active.

The function checks if the given `id` is within the range of `ids` that correspond
to the active validator set. The active validator set is defined as the most
recent `ACTIVE_VALIDATOR_SET_SIZE` number of validators.

### isCallerValidator

```js
function isCallerValidator() external view returns (bool) {
    return isValidator(msg.sender);
}
```

This function is a convenience function that allows anyone to check if the
caller of the function is a validator. It indicates whether the caller is a
validator. The function does this by calling `isValidator` and
passing in the caller's `address` as an argument.

### isCallerActiveValidator

```js
function isCallerActiveValidator() external view returns (bool) {
    return isValidatorActive(getValidatorId(msg.sender));
}
```

This function is a convenience function that allows anyone to check
if the caller of the function is an active validator. It indicates whether
the caller is an active validator. The function does this by calling
`isValidatorActive` and passing in the caller's validator `id` as an
argument. The validator `id` is retrieved by calling `getValidatorId`
and passing in the caller's address as an argument.
