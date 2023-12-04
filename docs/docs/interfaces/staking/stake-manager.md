The `StakeManager` interface manages stakes for all childchains in the Edge architecture.

## Events

### ChildManagerRegistered()

```solidity
event ChildManagerRegistered(uint256 indexed id, address indexed manager);
```

This event is emitted when a new child chain manager is registered with the StakeManager contract. It includes the ID of the child chain and the address of the manager contract.

### StakeAdded()

```solidity
event StakeAdded(uint256 indexed id, address indexed validator, uint256 amount);
```

This event is emitted when a validator adds stake for a child chain. It includes the ID of the child chain, the address of the validator, and the amount of stake added.

### StakeRemoved()

```solidity
event StakeRemoved(uint256 indexed id, address indexed validator, uint256 amount);
```

This event is emitted when a validator's stake is removed for a child chain. It includes the ID of the child chain, the address of the validator, and the amount of stake removed.

### StakeWithdrawn()

```solidity
event StakeWithdrawn(address indexed validator, address indexed recipient, uint256 amount);
```

This event is emitted when a validator withdraws their released stake. It includes the address of the validator, the address of the recipient, and the amount of stake withdrawn.

### ValidatorSlashed()

```solidity
event ValidatorSlashed(uint256 indexed id, address indexed validator, uint256 amount);
```

This event is emitted when a validator's stake is slashed for a child chain. It includes the ID of the child chain, the address of the validator, and the amount of stake slashed.

## Functions

### registerChildChain()

```solidity
function registerChildChain(address manager) external returns (uint256 id);
```

This function is called to register a new child chain with the StakeManager contract. It associates the child chain with its manager contract.

#### Parameters:

`manager`: The address of the manager contract for the child chain.

### stakeFor()

```solidity
function releaseStakeOf(address validator, uint256 amount) external;
```

This function is called by a validator to stake for a child chain. It adds the specified amount of stake for the given child chain.

#### Parameters:

- `id`: The ID of the child chain.
- `amount`: The amount of stake to be added.

### releaseStakeOf()

```solidity
function releaseStakeOf(address validator, uint256 amount) external;
```

This function is called by the child manager contract to release a validator's stake. It releases the specified amount of stake for the given validator.

#### Parameters:

- `validator`: The address of the validator.
- `amount`: The amount of stake to be released.

### withdrawStake()

```solidity
function withdrawStake(address to, uint256 amount) external;
```

This function allows a validator to withdraw their released stake. It transfers the specified amount of released stake to the specified recipient address.

#### Parameters:

- `to`: The address where the withdrawn stake will be sent.
- `amount`: The amount of released stake to be withdrawn.

### slashStakeOf()

```solidity
function slashStakeOf(address validator, uint256 amount) external;
```

This function is called by the child manager contract to slash a validator's stake. It slashes the specified amount of stake for the given validator, and the manager contract collects the slashed amount.

#### Parameters:

- `validator`: The address of the validator.
- `amount`: The amount of stake to be slashed.

### withdrawableStake()

```solidity
function withdrawableStake(address validator) external view returns (uint256 amount);
```

This function returns the amount of stake that a validator can withdraw. It represents the released stake that is available for withdrawal.

#### Parameters:

`validator`: The address of the validator.

#### Returns:

`amount`: The amount of stake that the validator can withdraw.

### totalStake()

```solidity
function totalStake() external view returns (uint256 amount);
```

This function returns the total amount of stake staked for all child chains.

#### Returns:

`amount`: The total amount of stake staked for all child chains.

### totalStakeOfChild()

```solidity
function totalStakeOfChild(uint256 id) external view returns (uint256 amount);
```

This function returns the total amount of stake staked for a specific child chain.

#### Parameters:

`id`: The ID of the child chain.

#### Returns:

`amount`: The total amount of stake staked for the specified child chain.

### totalStakeOf()

```solidity
function totalStakeOf(address validator) external view returns (uint256 amount);
```

This function returns the total amount of stake staked by a validator for all child chains.

#### Parameters:

`validator`: The address of the validator.

#### Returns:

`amount`: The total amount of stake staked by the specified validator.

### stakeOf()

```solidity
function stakeOf(address validator, uint256 id) external view returns (uint256 amount);
```

This function returns the amount of stake staked by a validator for a specific child chain.

#### Parameters:

- `validator`: The address of the validator.
- `id`: The ID of the child chain.

#### Returns:

`amount`: The amount of stake staked by the specified validator for the specified child chain.

### managerOf()

```solidity
function managerOf(uint256 id) external view returns (ISupernetManager manager);
```

This function returns the child chain manager contract for a specific child chain.

#### Parameters:

`id`: The ID of the child chain.

#### Returns:

`manager`: The address of the child chain manager contract.

### idFor()

```solidity
function idFor(address manager) external view returns (uint256 id);
```

This function returns the child chain ID associated with a specific child chain manager contract.

#### Parameters:

`manager`: The address of the child chain manager contract.

#### Returns:

`id`: The ID of the child chain associated with the specified manager contract.
