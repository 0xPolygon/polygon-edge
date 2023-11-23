The `IRootValidatorSet` interface provides functionality for managing the validator set on the rootchain. It allows adding new validators and querying validator information. This user guide will explain how to interact with the functions provided by the `IRootValidatorSet` interface.

## Data Structures

### Validator

The Validator struct represents a validator in the validator set.

- `_address` (address): The Ethereum address of the validator.
- `blsKey` (uint256[4]): The BLS public key of the validator.

## Functions

### addValidators()

This function adds new validators to the validator set.

#### Parameters

`newValidators` (Validator[]): An array of Validator structs representing the new validators to be added.

#### Usage

To add new validators to the validator set, call the `addValidators()` function with the required parameters:

```solidity
IRootValidatorSet rootValidatorSetInstance = IRootValidatorSet(rootValidatorSetAddress);

IRootValidatorSet.Validator[] memory newValidators = new IRootValidatorSet.Validator[](2);

newValidators[0] = IRootValidatorSet.Validator(validatorAddress1, blsKey1);
newValidators[1] = IRootValidatorSet.Validator(validatorAddress2, blsKey2);

rootValidatorSetInstance.addValidators(newValidators);
```

Replace `rootValidatorSetAddress` with the address of an existing `IRootValidatorSet` implementation. Replace `validatorAddress1`, `blsKey1`, `validatorAddress2`, and `blsKey2` with the Ethereum addresses and BLS public keys of the new validators.

### getValidatorBlsKey()

This function returns the BLS public key of a validator by their ID.

#### Parameters

`id` (uint256): The ID of the validator.

#### Usage

To get the BLS public key of a validator by their ID, call the `getValidatorBlsKey()` function with the required parameter:

```solidity
IRootValidatorSet rootValidatorSetInstance = IRootValidatorSet(rootValidatorSetAddress);

uint256[4] memory blsKey = rootValidatorSetInstance.getValidatorBlsKey(validatorId);
```

Replace rootValidatorSetAddress with the address of an existing `IRootValidatorSet` implementation. Replace `validatorId` with the ID of the validator whose BLS public key you want to retrieve.

### activeValidatorSetSize()

This function returns the size of the active validator set.

#### Usage

To get the size of the active validator set, call the `activeValidatorSetSize()` function:

```solidity
IRootValidatorSet rootValidatorSetInstance = IRootValidatorSet(rootValidatorSetAddress);

uint256 validatorSetSize = rootValidatorSetInstance.activeValidatorSetSize();
```

Replace rootValidatorSetAddress with the address of an existing `IRootValidatorSet` implementation. The function will return the number of active validators in the validator set.
