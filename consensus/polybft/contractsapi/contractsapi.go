package contractsapi

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	_ = big.NewInt
	_ = types.Address{}
	_ = ethgo.Log{}
)

var (
	commitMethodType = abi.MustNewMethod("function commit(tuple(uint256 startId,uint256 endId,uint256 leaves,bytes32 root) bundle,bytes signature,bytes bitmap)") //nolint:all
)

type Bundle struct {
	StartID *big.Int `abi:"startId"`
	EndID   *big.Int `abi:"endId"`
	Leaves  *big.Int `abi:"leaves"`
	Root    [32]byte `abi:"root"`
}

type CommitMethod struct {
	Bundle    Bundle `abi:"bundle"`
	Signature []byte `abi:"signature"`
	Bitmap    []byte `abi:"bitmap"`
}

func (c *CommitMethod) EncodeAbi() ([]byte, error) {
	return commitMethodType.Encode(c)
}

func (c *CommitMethod) DecodeAbi(buf []byte) error {
	return decodeMethod(commitMethodType, buf, c)
}

var (
	commitEpochMethodType = abi.MustNewMethod("function commitEpoch(uint256 id,tuple(uint256 startBlock,uint256 endBlock,bytes32 epochRoot) epoch,tuple(uint256 epochId,tuple(address validator,uint256 signedBlocks)[] uptimeData,uint256 totalBlocks) uptime)") //nolint:all
)

type Epoch struct {
	StartBlock *big.Int `abi:"startBlock"`
	EndBlock   *big.Int `abi:"endBlock"`
	EpochRoot  [32]byte `abi:"epochRoot"`
}

type UptimeData struct {
	Validator    types.Address `abi:"validator"`
	SignedBlocks *big.Int      `abi:"signedBlocks"`
}

type Uptime struct {
	EpochID     *big.Int     `abi:"epochId"`
	UptimeData  []UptimeData `abi:"uptimeData"`
	TotalBlocks *big.Int     `abi:"totalBlocks"`
}

type CommitEpochMethod struct {
	ID     *big.Int `abi:"id"`
	Epoch  Epoch    `abi:"epoch"`
	Uptime Uptime   `abi:"uptime"`
}

func (c *CommitEpochMethod) EncodeAbi() ([]byte, error) {
	return commitEpochMethodType.Encode(c)
}

func (c *CommitEpochMethod) DecodeAbi(buf []byte) error {
	return decodeMethod(commitEpochMethodType, buf, c)
}

var (
	StateSyncedEventType = abi.MustNewEvent("event StateSynced(uint256 indexed id,address indexed sender,address indexed receiver,bytes data)") //nolint:all
)

type StateSyncedEvent struct {
	ID       *big.Int      `abi:"id"`
	Sender   types.Address `abi:"sender"`
	Receiver types.Address `abi:"receiver"`
	Data     []byte        `abi:"data"`
}

func (S *StateSyncedEvent) ParseLog(log *ethgo.Log) error {
	return decodeEvent(StateSyncedEventType, log, S)
}
