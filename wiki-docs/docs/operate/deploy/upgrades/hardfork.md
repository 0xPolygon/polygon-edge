
Drawing from a real-world example, this guide delves into the process of introducing a new feature via a hardfork in an Edge-powered chain, utilizing a fork manager to manage the activation and behavior of the fork.

## 1. Adding Fields

Add the `ExtraData` field to Extra. Before block 100, the field will be ignored. After block 100, it will be used. Your Extra struct should look like this:

```go
type Extra struct {
	Validators  *validator.ValidatorSetDelta
	Parent      *Signature
	Committed   *Signature
	Checkpoint  *CheckpointData
	ExtraData   []byte // NewField
	BlockNumber uint64 // field used by forking manager
}
```

The `ExtraData` field is a byte slice (`[]byte`), which can hold any kind of data.

## 2. Implement Extra.Marshal() Behavior

The marshaling process will need to account for the ExtraData field after block 100.

```go
type ExtraHandlerNewField struct {
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (e *ExtraHandlerNewField) MarshalRLPWith(extra *Extra, ar *fastrlp.Arena) *fastrlp.Value {
	vv := extraMarshalBaseFields(extra, ar, ar.NewArray())
	vv.Set(ar.NewBytes(extra.ExtraData))
	return vv
}
```

This code creates a new type `ExtraHandlerNewField` which has a method `MarshalRLPWith`. This method handles the marshaling of the `Extra` struct and specifically takes into account the `ExtraData` field.

## 3. Register the Fork in chain/params.go

`chain/params.go` is where the details about the fork are specified, such as its name, when it is activated, and how to determine its activity at a given block.

### 3.1 Define the Fork’s Name

We first define a constant string to represent the name of the fork.

```go
const (
...
	NewFieldForkName     = "newFieldFork"
)
```

### 3.2 Add the Fork as a Supported Fork

We then include the fork in the `ForksInTime` struct to mark it as a supported fork.

```go
type ForksInTime struct {
...
	NewFieldFork bool
}
```

### 3.3 Include the Fork into the genesis.json

Next, we include the fork in the `genesis.json`.

```go
var AllForksEnabled = &Forks{
...
	NewFieldForkName:    NewFork(0),
}
```

### 3.4 Add Support to Read if the Fork is Active at the Current Block

Finally, we add the ability to read if the fork is active at a specific block.

```go
func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
...
		NewFieldFork:    f.IsActive(NewFieldForkName, block),
	}
}
```

## 4. Register Fork Handlers

Fork handlers are the functions that are run when a fork becomes active. Here, we register a handler for our new fork, as well as a default handler for blocks before the fork.

```
func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
...
		NewFieldFork:    f.IsActive(NewFieldForkName, block),
	}
}
```

## 5. Fork Management

In the `chain/params.go` file, define the parameters related to the fork. They should always be pointer values. For example:

```go
type ForkParams struct {
	MaxValidatorSetSize *uint64 `json:"maxValidatorSetSize,omitempty"`
    ...
}
```

These parameters represent new rules or behaviors to be introduced.

## 6. Configuring the Fork in genesis.json

In the genesis.json file, specify all available forks and their parameters. For instance:

```json
"forks": {
    "EIP150": {
        "block": 0
    },
    ...
    "myFork": {
        "block": 10,
        "params": {
            "maxValidatorSetSize": 20
        }
    },
}
```

Here, a new fork named `myFork` is added with specific parameters and values. For this fork, `maxValidatorSetSize` will be 20.

## 7. Accessing and Using Fork Parameters in Code

You can access these parameters in the code using the fork manager. For example:

```go
maxValidatorSetSize := 100 // default value
ps := forkmanager.GetInstance().GetParam(blockNumber)
if ps != nil {
    maxValidatorSetSize = ps.MaxValidatorSetSize
}
```

For `blockNumber` < 10, `maxValidatorSetSize` will be set to 100. For `blockNumber` >= 10, the `maxValidatorSetSize` will be 20, the value specified for the `myFork` fork.

## 8. Setting Default Parameter Values

If you don't want to use if statements in the code and always want to have default values for the parameters, change the consensus factory method defined in `server/builtin.go`:

```go
func ForkManagerInitialParamsFactory(config *chain.Chain) (*chain.ForkParams, error) {
	pbftConfig, err := GetPolyBFTConfig(config)
	if err != nil {
		return nil, err
	}
​
	return &chain.ForkParams{
		MaxValidatorSetSize: &pbftConfig.MaxValidatorSetSize,
        ...
	}, nil
}
```

Each fork doesn't need to specify all parameters, only the ones that have changed between forks. Other parameters take the value from the previous fork.

## 9. Network Upgrade

The introduction of a hard fork necessitates an upgrade process across the network. Nodes need to update their software to incorporate the new fork code, and the new `ExtraData` field becomes active upon reaching the designated block height. Here's an example of the step-by-step process that follows the example used above:

1. **Generate New Genesis File**: Create a new genesis.json using the updated Edge implementation that includes the fork.
2. **Edit Activation Block**: Set the fork's activation block to 100 in the genesis.json file, as in "MyFirstForkName": 100.
3. **Distribute Genesis File**: Share the updated genesis.json file with all network nodes.

    For a step-by-step guide on generating a new genesis file, please consult the chain configuration deployment guide 
    [<ins>here</ins>](../genesis.md).

4. **Stop Client**: Pause the Edge client on all network nodes before block 100.
5. **Replace Files**: Replace the existing genesis.json and Edge files with the updated versions.
6. **Restart Edge**: Restart the Edge client to apply the hard fork updates.

Remember, communication with network participants about the specific process and timeline is crucial for a successful upgrade.
