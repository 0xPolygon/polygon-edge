The `CustomSupernetManager` contract manages validator access and syncs voting power between the stake manager and the validator set on the child chain. It implements the base `SupernetManager` contract.

## Events

### AddedToWhitelist()

```solidity
event AddedToWhitelist(address indexed validator);
```

This event is emitted when a validator is added to the whitelist of validators allowed to stake.

### RemovedFromWhitelist()

```solidity
event RemovedFromWhitelist(address indexed validator);
```

This event is emitted when a validator is removed from the whitelist of validators allowed to stake.

### ValidatorRegistered()

```solidity
event ValidatorRegistered(address indexed validator, uint256[4] blsKey);
```

This event is emitted when a validator is registered with their public key.

### ValidatorDeactivated()

```solidity
event ValidatorDeactivated(address validator);
```

This event is emitted when a validator is deactivated.

### GenesisFinalized()

```solidity
event GenesisFinalized(uint256 amountValidators);
```

This event is emitted when the initial genesis validator set is finalized.

### StakingEnabled()

```solidity
event StakingEnabled();
```

This event is emitted when staking is enabled after the successful initialization of the child chain.

## Functions

### whitelistValidators()

```solidity
function whitelistValidators(address[] calldata validators_) external;
```

This function allows whitelisting validators that are allowed to stake. It can only be called by the owner of the contract.

### Parameters:

`validators_`: An array of addresses representing the validators to be whitelisted.

### register()

```solidity
function register(uint256[2] calldata signature, uint256[4] calldata pubkey) external;
```

This function is called to register the public key of a validator. It validates the signature and registers the validator with their public key.

#### Parameters:

- `signature`: An array of two 256-bit integers representing the signature.
- `pubkey`: An array of four 256-bit integers representing the public key of the validator.

### finalizeGenesis()

```solidity
function finalizeGenesis() external;
```

This function is called to finalize the initial genesis validator set. It can only be called by the owner of the contract.

### enableStaking()

```solidity
 function enableStaking() external;
```

This function is called to enable staking after the successful initialization of the child chain. It can only be called by the owner of the contract.

### withdrawSlashedStake()

```solidity
function withdrawSlashedStake(address to) external;
```

This function is called to withdraw slashed MATIC of slashed validators. It can only be called by the owner of the contract.

#### Parameters:

`to`: The address where the slashed stake will be withdrawn.

### onL2StateReceive()

```solidity
function onL2StateReceive(uint256 /*id*/, address sender, bytes calldata data) external;
```

This function is called by the exit helpers to either release the stake of a validator or slash it. It can only be called after the genesis and must be synced from the child chain.

#### Parameters:

- `sender`: The address of the sender calling the function.
- `data`: The data containing the information about the stake release or slash.

### genesisSet()

```solidity
function genesisSet() external view returns (GenesisValidator[] memory);
```

This function returns the initial genesis validator set with their balances.

#### Returns:

`GenesisValidator[]`: An array of GenesisValidator structs representing the initial genesis validator set.

### getValidator()

```solidity
function getValidator(address validator_) external view returns (Validator memory);
```

This function returns the Validator instance based on the provided validator address.

#### Parameters:

`validator_`: The address of the validator.

#### Returns:

`Validator`: The Validator struct representing the validator instance.
