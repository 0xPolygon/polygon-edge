// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package CentrifugeAsset

import (
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// CentrifugeAssetABI is the input ABI used to generate the binding from.
const CentrifugeAssetABI = "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"asset\",\"type\":\"bytes32\"}],\"name\":\"AssetStored\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"_assetsStored\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"asset\",\"type\":\"bytes32\"}],\"name\":\"store\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]"

// CentrifugeAssetBin is the compiled bytecode used for deploying new contracts.
var CentrifugeAssetBin = "0x608060405234801561001057600080fd5b5061029e806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c8063654cf88c1461003b57806396add60014610057575b600080fd5b61005560048036038101906100509190610177565b610087565b005b610071600480360381019061006c9190610177565b610142565b60405161007e91906101ef565b60405180910390f35b60008082815260200190815260200160002060009054906101000a900460ff16156100e7576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016100de9061020a565b60405180910390fd5b600160008083815260200190815260200160002060006101000a81548160ff021916908315150217905550807f08ae553713effae7116be03743b167b8b803449ee8fb912c2ec43dc2c824f53560405160405180910390a250565b60006020528060005260406000206000915054906101000a900460ff1681565b60008135905061017181610251565b92915050565b60006020828403121561018957600080fd5b600061019784828501610162565b91505092915050565b6101a98161023b565b82525050565b60006101bc60178361022a565b91507f617373657420697320616c72656164792073746f7265640000000000000000006000830152602082019050919050565b600060208201905061020460008301846101a0565b92915050565b60006020820190508181036000830152610223816101af565b9050919050565b600082825260208201905092915050565b60008115159050919050565b6000819050919050565b61025a81610247565b811461026557600080fd5b5056fea2646970667358221220635e7023f0d6831255c9d3247bcb04f1d174ac7a45614bc3e812928f8814ca9564736f6c63430006040033"

// DeployCentrifugeAsset deploys a new Ethereum contract, binding an instance of CentrifugeAsset to it.
func DeployCentrifugeAsset(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *CentrifugeAsset, error) {
	parsed, err := abi.JSON(strings.NewReader(CentrifugeAssetABI))
	if err != nil {
		return common.Address{}, nil, nil, err
	}

	address, tx, contract, err := bind.DeployContract(auth, parsed, common.FromHex(CentrifugeAssetBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &CentrifugeAsset{CentrifugeAssetCaller: CentrifugeAssetCaller{contract: contract}, CentrifugeAssetTransactor: CentrifugeAssetTransactor{contract: contract}, CentrifugeAssetFilterer: CentrifugeAssetFilterer{contract: contract}}, nil
}

// CentrifugeAsset is an auto generated Go binding around an Ethereum contract.
type CentrifugeAsset struct {
	CentrifugeAssetCaller     // Read-only binding to the contract
	CentrifugeAssetTransactor // Write-only binding to the contract
	CentrifugeAssetFilterer   // Log filterer for contract events
}

// CentrifugeAssetCaller is an auto generated read-only Go binding around an Ethereum contract.
type CentrifugeAssetCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CentrifugeAssetTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CentrifugeAssetTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CentrifugeAssetFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CentrifugeAssetFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CentrifugeAssetSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CentrifugeAssetSession struct {
	Contract     *CentrifugeAsset  // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// CentrifugeAssetCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CentrifugeAssetCallerSession struct {
	Contract *CentrifugeAssetCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts          // Call options to use throughout this session
}

// CentrifugeAssetTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CentrifugeAssetTransactorSession struct {
	Contract     *CentrifugeAssetTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// CentrifugeAssetRaw is an auto generated low-level Go binding around an Ethereum contract.
type CentrifugeAssetRaw struct {
	Contract *CentrifugeAsset // Generic contract binding to access the raw methods on
}

// CentrifugeAssetCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CentrifugeAssetCallerRaw struct {
	Contract *CentrifugeAssetCaller // Generic read-only contract binding to access the raw methods on
}

// CentrifugeAssetTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CentrifugeAssetTransactorRaw struct {
	Contract *CentrifugeAssetTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCentrifugeAsset creates a new instance of CentrifugeAsset, bound to a specific deployed contract.
func NewCentrifugeAsset(address common.Address, backend bind.ContractBackend) (*CentrifugeAsset, error) {
	contract, err := bindCentrifugeAsset(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CentrifugeAsset{CentrifugeAssetCaller: CentrifugeAssetCaller{contract: contract}, CentrifugeAssetTransactor: CentrifugeAssetTransactor{contract: contract}, CentrifugeAssetFilterer: CentrifugeAssetFilterer{contract: contract}}, nil
}

// NewCentrifugeAssetCaller creates a new read-only instance of CentrifugeAsset, bound to a specific deployed contract.
func NewCentrifugeAssetCaller(address common.Address, caller bind.ContractCaller) (*CentrifugeAssetCaller, error) {
	contract, err := bindCentrifugeAsset(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CentrifugeAssetCaller{contract: contract}, nil
}

// NewCentrifugeAssetTransactor creates a new write-only instance of CentrifugeAsset, bound to a specific deployed contract.
func NewCentrifugeAssetTransactor(address common.Address, transactor bind.ContractTransactor) (*CentrifugeAssetTransactor, error) {
	contract, err := bindCentrifugeAsset(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CentrifugeAssetTransactor{contract: contract}, nil
}

// NewCentrifugeAssetFilterer creates a new log filterer instance of CentrifugeAsset, bound to a specific deployed contract.
func NewCentrifugeAssetFilterer(address common.Address, filterer bind.ContractFilterer) (*CentrifugeAssetFilterer, error) {
	contract, err := bindCentrifugeAsset(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CentrifugeAssetFilterer{contract: contract}, nil
}

// bindCentrifugeAsset binds a generic wrapper to an already deployed contract.
func bindCentrifugeAsset(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(CentrifugeAssetABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CentrifugeAsset *CentrifugeAssetRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CentrifugeAsset.Contract.CentrifugeAssetCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CentrifugeAsset *CentrifugeAssetRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.CentrifugeAssetTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CentrifugeAsset *CentrifugeAssetRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.CentrifugeAssetTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CentrifugeAsset *CentrifugeAssetCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CentrifugeAsset.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CentrifugeAsset *CentrifugeAssetTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CentrifugeAsset *CentrifugeAssetTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.contract.Transact(opts, method, params...)
}

// AssetsStored is a free data retrieval call binding the contract method 0x96add600.
//
// Solidity: function _assetsStored(bytes32 ) view returns(bool)
func (_CentrifugeAsset *CentrifugeAssetCaller) AssetsStored(opts *bind.CallOpts, arg0 [32]byte) (bool, error) {
	var out []interface{}
	err := _CentrifugeAsset.contract.Call(opts, &out, "_assetsStored", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// AssetsStored is a free data retrieval call binding the contract method 0x96add600.
//
// Solidity: function _assetsStored(bytes32 ) view returns(bool)
func (_CentrifugeAsset *CentrifugeAssetSession) AssetsStored(arg0 [32]byte) (bool, error) {
	return _CentrifugeAsset.Contract.AssetsStored(&_CentrifugeAsset.CallOpts, arg0)
}

// AssetsStored is a free data retrieval call binding the contract method 0x96add600.
//
// Solidity: function _assetsStored(bytes32 ) view returns(bool)
func (_CentrifugeAsset *CentrifugeAssetCallerSession) AssetsStored(arg0 [32]byte) (bool, error) {
	return _CentrifugeAsset.Contract.AssetsStored(&_CentrifugeAsset.CallOpts, arg0)
}

// Store is a paid mutator transaction binding the contract method 0x654cf88c.
//
// Solidity: function store(bytes32 asset) returns()
func (_CentrifugeAsset *CentrifugeAssetTransactor) Store(opts *bind.TransactOpts, asset [32]byte) (*types.Transaction, error) {
	return _CentrifugeAsset.contract.Transact(opts, "store", asset)
}

// Store is a paid mutator transaction binding the contract method 0x654cf88c.
//
// Solidity: function store(bytes32 asset) returns()
func (_CentrifugeAsset *CentrifugeAssetSession) Store(asset [32]byte) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.Store(&_CentrifugeAsset.TransactOpts, asset)
}

// Store is a paid mutator transaction binding the contract method 0x654cf88c.
//
// Solidity: function store(bytes32 asset) returns()
func (_CentrifugeAsset *CentrifugeAssetTransactorSession) Store(asset [32]byte) (*types.Transaction, error) {
	return _CentrifugeAsset.Contract.Store(&_CentrifugeAsset.TransactOpts, asset)
}

// CentrifugeAssetAssetStoredIterator is returned from FilterAssetStored and is used to iterate over the raw logs and unpacked data for AssetStored events raised by the CentrifugeAsset contract.
type CentrifugeAssetAssetStoredIterator struct {
	Event *CentrifugeAssetAssetStored // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *CentrifugeAssetAssetStoredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CentrifugeAssetAssetStored)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(CentrifugeAssetAssetStored)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *CentrifugeAssetAssetStoredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CentrifugeAssetAssetStoredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CentrifugeAssetAssetStored represents a AssetStored event raised by the CentrifugeAsset contract.
type CentrifugeAssetAssetStored struct {
	Asset [32]byte
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterAssetStored is a free log retrieval operation binding the contract event 0x08ae553713effae7116be03743b167b8b803449ee8fb912c2ec43dc2c824f535.
//
// Solidity: event AssetStored(bytes32 indexed asset)
func (_CentrifugeAsset *CentrifugeAssetFilterer) FilterAssetStored(opts *bind.FilterOpts, asset [][32]byte) (*CentrifugeAssetAssetStoredIterator, error) {

	var assetRule []interface{}
	for _, assetItem := range asset {
		assetRule = append(assetRule, assetItem)
	}

	logs, sub, err := _CentrifugeAsset.contract.FilterLogs(opts, "AssetStored", assetRule)
	if err != nil {
		return nil, err
	}
	return &CentrifugeAssetAssetStoredIterator{contract: _CentrifugeAsset.contract, event: "AssetStored", logs: logs, sub: sub}, nil
}

// WatchAssetStored is a free log subscription operation binding the contract event 0x08ae553713effae7116be03743b167b8b803449ee8fb912c2ec43dc2c824f535.
//
// Solidity: event AssetStored(bytes32 indexed asset)
func (_CentrifugeAsset *CentrifugeAssetFilterer) WatchAssetStored(opts *bind.WatchOpts, sink chan<- *CentrifugeAssetAssetStored, asset [][32]byte) (event.Subscription, error) {

	var assetRule []interface{}
	for _, assetItem := range asset {
		assetRule = append(assetRule, assetItem)
	}

	logs, sub, err := _CentrifugeAsset.contract.WatchLogs(opts, "AssetStored", assetRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CentrifugeAssetAssetStored)
				if err := _CentrifugeAsset.contract.UnpackLog(event, "AssetStored", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAssetStored is a log parse operation binding the contract event 0x08ae553713effae7116be03743b167b8b803449ee8fb912c2ec43dc2c824f535.
//
// Solidity: event AssetStored(bytes32 indexed asset)
func (_CentrifugeAsset *CentrifugeAssetFilterer) ParseAssetStored(log types.Log) (*CentrifugeAssetAssetStored, error) {
	event := new(CentrifugeAssetAssetStored)
	if err := _CentrifugeAsset.contract.UnpackLog(event, "AssetStored", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
