package bridge

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/helper/keccak"
	"github.com/0xPolygon/polygon-sdk/types"
)

func SetupPredeployedContract(
	premineMap map[types.Address]*chain.GenesisAccount,
	chainID uint64,
) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	if err := PredeloyBridgeContract(DefaultBridgeContractAddress, DefaultERC20HandlerContractAddress, premineMap, chainID, config); err != nil {
		return err
	}
	if err := PredeloyERC20HandlerContract(DefaultERC20HandlerContractAddress, DefaultBridgeContractAddress, DefaultERC20ContractAddress, premineMap, config); err != nil {
		return err
	}
	if err := PredeloyERC20Contract(DefaultERC20ContractAddress, premineMap, config); err != nil {
		return err
	}

	return nil
}

func PredeloyBridgeContract(
	address types.Address,
	erc20HandlerAddress types.Address,
	premineMap map[types.Address]*chain.GenesisAccount,
	chainID uint64,
	config *Config,
) error {
	scHex, _ := hex.DecodeHex(BridgeContractByteCode)
	bridgeContract := &chain.GenesisAccount{
		Code: scHex,
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)

	// parse addresses
	adminAddress := types.StringToAddress(config.Genesis.Admin)
	fmt.Printf("adminAddress %+v\n", adminAddress)

	relayers := make([]types.Address, 0, len(config.Genesis.Relayers))
	for _, r := range config.Genesis.Relayers {
		r := types.StringToAddress(r)
		relayers = append(relayers, r)
		fmt.Printf("relayer %+v\n", r)
	}

	// slot0, _paused(bool)
	storageMap[types.BytesToHash(big.NewInt(0).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// slot1, _roles(mapping byte32 -> RoleData)
	storageMap[types.BytesToHash(big.NewInt(1).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// slot2, _chainID(uint8)
	bigChainID := big.NewInt(int64(chainID))
	storageMap[types.BytesToHash(big.NewInt(2).Bytes())] = types.BytesToHash(
		bigChainID.Bytes(),
	)

	// slot3, _relayerThreshold(uint256)
	relayerThereshold := big.NewInt(config.Genesis.RelayerThreshold)
	storageMap[types.BytesToHash(big.NewInt(3).Bytes())] = types.BytesToHash(
		relayerThereshold.Bytes(),
	)

	// slot4, _totalRelayers(uint256)
	totalRelayers := big.NewInt(int64(len(relayers)))
	storageMap[types.BytesToHash(big.NewInt(4).Bytes())] = types.BytesToHash(
		totalRelayers.Bytes(),
	)

	// slot6, _fee(uint256)
	fee := big.NewInt(0)
	storageMap[types.BytesToHash(big.NewInt(6).Bytes())] = types.BytesToHash(
		fee.Bytes(),
	)

	// slot7, _expiry(uint256)
	expiry := big.NewInt(100)
	storageMap[types.BytesToHash(big.NewInt(7).Bytes())] = types.BytesToHash(
		expiry.Bytes(),
	)

	// slot9, _resourceIDToHandlerAddress(mapping bytes32 => address)
	storageMap[types.BytesToHash(big.NewInt(9).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// roles
	getRoleIndex := func(role []byte) []byte {
		return common.PadLeftOrTrim(
			keccak.Keccak256(nil, append(
				role,
				// uint256 (slot)
				common.PadLeftOrTrim(big.NewInt(1).Bytes(), 32)...,
			)),
			32,
		)
	}

	defaultAdminRoleIndex := getRoleIndex(DefaultAdminRole)
	relayerRoleIndex := getRoleIndex(RelayerRole)

	// _roles
	// grant for deployer
	// array
	// set size
	storageMap[types.BytesToHash(defaultAdminRoleIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)
	baseArrayIndex := keccak.Keccak256(nil, defaultAdminRoleIndex)
	storageMap[types.BytesToHash(baseArrayIndex)] = types.BytesToHash(
		adminAddress[:],
	)
	// mapping
	baseMappingIndex := common.PadLeftOrTrim(
		new(big.Int).Add(
			big.NewInt(1),
			new(big.Int).SetBytes(defaultAdminRoleIndex),
		).Bytes(),
		32,
	)
	storageMap[types.BytesToHash(baseMappingIndex)] = types.BytesToHash(
		big.NewInt(0).Bytes(),
	)
	firstMemberIndex := common.PadLeftOrTrim(
		keccak.Keccak256(nil, append(
			common.PadLeftOrTrim(adminAddress[:], 32),
			baseMappingIndex...,
		)), 32)
	storageMap[types.BytesToHash(firstMemberIndex)] = types.BytesToHash(
		// one-indexed
		big.NewInt(1).Bytes(),
	)

	// grant for relayers
	// set size
	storageMap[types.BytesToHash(relayerRoleIndex)] = types.BytesToHash(
		new(big.Int).SetInt64(int64(len(relayers))).Bytes(),
	)
	baseArrayIndex = keccak.Keccak256(nil, relayerRoleIndex)
	baseMappingIndex = common.PadLeftOrTrim(
		new(big.Int).Add(
			big.NewInt(1),
			new(big.Int).SetBytes(relayerRoleIndex),
		).Bytes(),
		32,
	)
	storageMap[types.BytesToHash(baseMappingIndex)] = types.BytesToHash(
		big.NewInt(0).Bytes(),
	)
	grantRelayerRole := func(index int, address types.Address) {
		// array
		storageMap[types.BytesToHash(common.PadLeftOrTrim(new(big.Int).Add(
			big.NewInt(int64(index)),
			new(big.Int).SetBytes(baseArrayIndex),
		).Bytes(), 32))] = types.BytesToHash(address[:])
		// map
		mapIndex := common.PadLeftOrTrim(
			keccak.Keccak256(nil, append(
				common.PadLeftOrTrim(address[:], 32),
				baseMappingIndex...,
			)), 32)
		storageMap[types.BytesToHash(mapIndex)] = types.BytesToHash(
			// one-indexed
			big.NewInt(int64(index) + 1).Bytes(),
		)
	}
	for idx, val := range relayers {
		grantRelayerRole(idx, val)
	}

	// slot2
	adminRoleIndexForRelayerRole := new(big.Int).Add(
		new(big.Int).SetBytes(relayerRoleIndex),
		big.NewInt(2),
	)
	storageMap[types.BytesToHash(adminRoleIndexForRelayerRole.Bytes())] = types.BytesToHash(
		DefaultAdminRole,
	)

	// _resouceIDToHandlerAddress
	// erc20
	erc20ResouceIndex := common.PadLeftOrTrim(
		keccak.Keccak256(nil, append(
			ERC20ResouceID[:],
			// uint256 (slot)
			common.PadLeftOrTrim(big.NewInt(9).Bytes(), 32)...,
		)),
		32,
	)
	storageMap[types.BytesToHash(erc20ResouceIndex)] = types.BytesToHash(
		erc20HandlerAddress[:],
	)

	// Save the storage map
	bridgeContract.Storage = storageMap

	// Add the account to the premine map so the executor can apply it to state
	premineMap[address] = bridgeContract

	fmt.Printf("Deploy Bridge Contract to %+v\n", address)

	return nil
}

func PredeloyERC20HandlerContract(
	address types.Address,
	bridgeContractAddress types.Address,
	tokenContractAddress types.Address,
	premineMap map[types.Address]*chain.GenesisAccount,
	config *Config,
) error {
	scHex, _ := hex.DecodeHex(ERC20HandlerByteCode)
	erc20HandlerContract := &chain.GenesisAccount{
		Code: scHex,
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)

	// slot0 _bridgeAddress(address)
	storageMap[types.BytesToHash(big.NewInt(0).Bytes())] = types.BytesToHash(
		bridgeContractAddress[:],
	)

	// slot1 _resourceIDToTokenContractAddress(mapping bytes32->address)
	storageMap[types.BytesToHash(big.NewInt(1).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// slot2 _tokenContractAddressToResourceID(mapping address->bytes32)
	storageMap[types.BytesToHash(big.NewInt(2).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// slot3 _contractWhitelist (mapping address -> bool)
	storageMap[types.BytesToHash(big.NewInt(3).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// slot4 _burnList (mapping address -> bool)
	storageMap[types.BytesToHash(big.NewInt(3).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)

	// _resourceIDToTokenContractAddress
	erc20ResouceIndex := keccak.Keccak256(nil, append(
		ERC20ResouceID[:],
		// uint256 (slot)
		common.PadLeftOrTrim(big.NewInt(1).Bytes(), 32)...,
	))
	storageMap[types.BytesToHash(erc20ResouceIndex)] = types.BytesToHash(
		tokenContractAddress[:],
	)

	tokenContractAddressToResourceIDIndex := keccak.Keccak256(nil, append(
		common.PadLeftOrTrim(tokenContractAddress[:], 32),
		// uint256 (slot)
		common.PadLeftOrTrim(big.NewInt(2).Bytes(), 32)...,
	))
	storageMap[types.BytesToHash(tokenContractAddressToResourceIDIndex)] = types.BytesToHash(
		ERC20ResouceID[:],
	)

	contractAddressToWhiteListIndex := keccak.Keccak256(nil, append(
		common.PadLeftOrTrim(tokenContractAddress[:], 32),
		// uint256 (slot)
		common.PadLeftOrTrim(big.NewInt(3).Bytes(), 32)...,
	))
	storageMap[types.BytesToHash(contractAddressToWhiteListIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)

	contractAddressToBurnListIndex := keccak.Keccak256(nil, append(
		common.PadLeftOrTrim(tokenContractAddress[:], 32),
		// uint256 (slot)
		common.PadLeftOrTrim(big.NewInt(4).Bytes(), 32)...,
	))
	storageMap[types.BytesToHash(contractAddressToBurnListIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)

	// Save the storage map
	erc20HandlerContract.Storage = storageMap

	// Add the account to the premine map so the executor can apply it to state
	premineMap[address] = erc20HandlerContract

	fmt.Printf("Deploy ERC20 Handler Contract to %+v\n", address)

	return nil
}

func PredeloyERC20Contract(
	address types.Address,
	premineMap map[types.Address]*chain.GenesisAccount,
	config *Config,
) error {
	scHex, _ := hex.DecodeHex(ERC20ByteCode)
	erc20Contract := &chain.GenesisAccount{
		Code: scHex,
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)

	// Save the storage map
	erc20Contract.Storage = storageMap

	// slot 0, roles
	storageMap[types.BytesToHash(big.NewInt(0).Bytes())] = types.BytesToHash(
		new(big.Int).Bytes(),
	)
	// set role
	adminAddress := types.StringToAddress(config.Genesis.Admin)

	getRoleIndex := func(role []byte) []byte {
		return common.PadLeftOrTrim(
			keccak.Keccak256(nil, append(
				role,
				// uint256 (slot)
				common.PadLeftOrTrim(big.NewInt(0).Bytes(), 32)...,
			)),
			32,
		)
	}

	// defaultAdminRole
	defaultAdminRoleIndex := getRoleIndex(DefaultAdminRole)
	storageMap[types.BytesToHash(defaultAdminRoleIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)
	baseArrayIndex := keccak.Keccak256(nil, defaultAdminRoleIndex)
	storageMap[types.BytesToHash(baseArrayIndex)] = types.BytesToHash(
		adminAddress[:],
	)
	// mapping
	baseMappingIndex := common.PadLeftOrTrim(
		new(big.Int).Add(
			big.NewInt(1),
			new(big.Int).SetBytes(defaultAdminRoleIndex),
		).Bytes(),
		32,
	)
	storageMap[types.BytesToHash(baseMappingIndex)] = types.BytesToHash(
		big.NewInt(0).Bytes(),
	)
	memberIndex := common.PadLeftOrTrim(
		keccak.Keccak256(nil, append(
			common.PadLeftOrTrim(adminAddress[:], 32),
			baseMappingIndex...,
		)), 32)
	storageMap[types.BytesToHash(memberIndex)] = types.BytesToHash(
		// one-indexed
		big.NewInt(1).Bytes(),
	)

	// minterRole
	minterRoleIndex := getRoleIndex(MinterRole)
	storageMap[types.BytesToHash(minterRoleIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)
	baseArrayIndex = keccak.Keccak256(nil, minterRoleIndex)
	storageMap[types.BytesToHash(baseArrayIndex)] = types.BytesToHash(
		adminAddress[:],
	)
	// mapping
	baseMappingIndex = common.PadLeftOrTrim(
		new(big.Int).Add(
			big.NewInt(1),
			new(big.Int).SetBytes(minterRoleIndex),
		).Bytes(),
		32,
	)
	storageMap[types.BytesToHash(baseMappingIndex)] = types.BytesToHash(
		big.NewInt(0).Bytes(),
	)
	memberIndex = common.PadLeftOrTrim(
		keccak.Keccak256(nil, append(
			common.PadLeftOrTrim(adminAddress[:], 32),
			baseMappingIndex...,
		)), 32)
	storageMap[types.BytesToHash(memberIndex)] = types.BytesToHash(
		// one-indexed
		big.NewInt(1).Bytes(),
	)

	// pauserRole
	pauserRoleIndex := getRoleIndex(PauserRole)
	storageMap[types.BytesToHash(pauserRoleIndex)] = types.BytesToHash(
		big.NewInt(1).Bytes(),
	)
	baseArrayIndex = keccak.Keccak256(nil, pauserRoleIndex)
	storageMap[types.BytesToHash(baseArrayIndex)] = types.BytesToHash(
		adminAddress[:],
	)
	// mapping
	baseMappingIndex = common.PadLeftOrTrim(
		new(big.Int).Add(
			big.NewInt(1),
			new(big.Int).SetBytes(pauserRoleIndex),
		).Bytes(),
		32,
	)
	storageMap[types.BytesToHash(baseMappingIndex)] = types.BytesToHash(
		big.NewInt(0).Bytes(),
	)
	memberIndex = common.PadLeftOrTrim(
		keccak.Keccak256(nil, append(
			common.PadLeftOrTrim(adminAddress[:], 32),
			baseMappingIndex...,
		)), 32)
	storageMap[types.BytesToHash(memberIndex)] = types.BytesToHash(
		// one-indexed
		big.NewInt(1).Bytes(),
	)

	// slot 4, name(string)
	// TODO: handle with more than 32 bytes string
	name := []byte("Polygon Token")
	nameData := common.PadLeftOrTrim(
		big.NewInt(int64(len(name))*2).Bytes(),
		32,
	)
	copy(nameData[:len(name)], name)
	storageMap[types.BytesToHash(big.NewInt(4).Bytes())] = types.BytesToHash(nameData)

	// slot 5, synbol(string)
	symbol := []byte("PTOK")
	symbolData := common.PadLeftOrTrim(
		big.NewInt(int64(len(symbol))*2).Bytes(),
		32,
	)
	copy(symbolData[:len(symbol)], symbol)
	storageMap[types.BytesToHash(big.NewInt(5).Bytes())] = types.BytesToHash(symbolData)

	// slot 6, decimals(uint8)
	decimal := 18
	storageMap[types.BytesToHash(big.NewInt(6).Bytes())] = types.BytesToHash(
		big.NewInt(int64(decimal)).Bytes(),
	)

	// Add the account to the premine map so the executor can apply it to state
	premineMap[address] = erc20Contract

	fmt.Printf("Deploy ERC20 Contract to %+v\n", address)

	return nil
}
