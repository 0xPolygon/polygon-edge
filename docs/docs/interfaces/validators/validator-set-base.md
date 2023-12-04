The `IChildValidatorSetBase` interface is part of the childchain. It handles validator registration, stake storage, and reward distribution. This user guide will explain the key components and functions of the interface.

## Structs

### InitStruct

InitStruct is used during the initialization of the contract.

- `epochReward` (uint256): The reward per epoch.
- `minStake` (uint256): The minimum amount a validator must stake.
- `minDelegation` (uint256): The minimum amount for delegating MATIC tokens.
- `epochSize` (uint256): The size of an epoch.

### ValidatorInit

ValidatorInit is used when initializing a validator.

- `addr` (address): The validator's address.
- `pubkey` (uint256[4]): The validator's public BLS key.
- `signature` (uint256[2]): The validator's signature.
- `stake` (uint256): The amount staked by the validator.

### DoubleSignerSlashingInput

DoubleSignerSlashingInput represents the information about double signers to be slashed along with signatures and bitmap.

- `epochId` (uint256): The ID of the epoch.
- `eventRoot` (bytes32): The event root.
- `currentValidatorSetHash` (bytes32): The current validator set hash.
- `nextValidatorSetHash` (bytes32): The next validator set hash.
- `blockHash` (bytes32): The block hash.
- `bitmap` (bytes): The bitmap.
- `signature` (bytes): The signature.

## Functions

### commitEpoch

Commits an epoch to the contract. Called by the Edge client.

- `id` (uint256): The ID of the epoch to be committed.
- `epoch` (Epoch): The epoch data to be committed.
- `uptime` (Uptime): The uptime data for the epoch being committed.

### commitEpochWithDoubleSignerSlashing

Commits an epoch and slashes double signers. Called by the Edge client.

- `curEpochId` (uint256): The ID of the epoch to be committed.
- `blockNumber` (uint256): The block number at which the double signer occurred.
- `pbftRound` (uint256): The round number at which the double signing occurred.
- `epoch` (Epoch): The epoch data to be committed.
- `uptime` (Uptime): The uptime data for the epoch being committed.
- `inputs` (DoubleSignerSlashingInput[]): Information about double signers to be slashed along with signatures and bitmap.

### getCurrentValidatorSet

Returns the addresses of active validators in the current epoch, sorted by total stake (self-stake + delegation).

### getEpochByBlock

Looks up an epoch by block number in O(log n) time.

- `blockNumber` (uint256): The block number.

### totalActiveStake

Calculates the total stake of active validators (self-stake + delegation).
