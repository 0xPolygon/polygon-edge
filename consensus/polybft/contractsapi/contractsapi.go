// Code generated by scapi/gen. DO NOT EDIT.
package contractsapi

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

type StateSyncCommitment struct {
	StartID *big.Int   `abi:"startId"`
	EndID   *big.Int   `abi:"endId"`
	Root    types.Hash `abi:"root"`
}

var StateSyncCommitmentABIType = abi.MustNewType("tuple(uint256 startId,uint256 endId,bytes32 root)")

func (s *StateSyncCommitment) EncodeAbi() ([]byte, error) {
	return StateSyncCommitmentABIType.Encode(s)
}

func (s *StateSyncCommitment) DecodeAbi(buf []byte) error {
	return decodeStruct(StateSyncCommitmentABIType, buf, &s)
}

type CommitFunction struct {
	Commitment *StateSyncCommitment `abi:"commitment"`
	Signature  []byte               `abi:"signature"`
	Bitmap     []byte               `abi:"bitmap"`
}

func (c *CommitFunction) EncodeAbi() ([]byte, error) {
	return StateReceiver.Abi.Methods["commit"].Encode(c)
}

func (c *CommitFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(StateReceiver.Abi.Methods["commit"], buf, c)
}

type StateSync struct {
	ID       *big.Int      `abi:"id"`
	Sender   types.Address `abi:"sender"`
	Receiver types.Address `abi:"receiver"`
	Data     []byte        `abi:"data"`
}

var StateSyncABIType = abi.MustNewType("tuple(uint256 id,address sender,address receiver,bytes data)")

func (s *StateSync) EncodeAbi() ([]byte, error) {
	return StateSyncABIType.Encode(s)
}

func (s *StateSync) DecodeAbi(buf []byte) error {
	return decodeStruct(StateSyncABIType, buf, &s)
}

type ExecuteFunction struct {
	Proof []types.Hash `abi:"proof"`
	Obj   *StateSync   `abi:"obj"`
}

func (e *ExecuteFunction) EncodeAbi() ([]byte, error) {
	return StateReceiver.Abi.Methods["execute"].Encode(e)
}

func (e *ExecuteFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(StateReceiver.Abi.Methods["execute"], buf, e)
}

type StateSyncResultEvent struct {
	Counter *big.Int `abi:"counter"`
	Status  bool     `abi:"status"`
	Message []byte   `abi:"message"`
}

func (s *StateSyncResultEvent) ParseLog(log *ethgo.Log) error {
	return decodeEvent(StateReceiver.Abi.Events["StateSyncResult"], log, s)
}

type NewCommitmentEvent struct {
	StartID *big.Int   `abi:"startId"`
	EndID   *big.Int   `abi:"endId"`
	Root    types.Hash `abi:"root"`
}

func (n *NewCommitmentEvent) ParseLog(log *ethgo.Log) error {
	return decodeEvent(StateReceiver.Abi.Events["NewCommitment"], log, n)
}

type Epoch struct {
	StartBlock *big.Int   `abi:"startBlock"`
	EndBlock   *big.Int   `abi:"endBlock"`
	EpochRoot  types.Hash `abi:"epochRoot"`
}

var EpochABIType = abi.MustNewType("tuple(uint256 startBlock,uint256 endBlock,bytes32 epochRoot)")

func (e *Epoch) EncodeAbi() ([]byte, error) {
	return EpochABIType.Encode(e)
}

func (e *Epoch) DecodeAbi(buf []byte) error {
	return decodeStruct(EpochABIType, buf, &e)
}

type UptimeData struct {
	Validator    types.Address `abi:"validator"`
	SignedBlocks *big.Int      `abi:"signedBlocks"`
}

var UptimeDataABIType = abi.MustNewType("tuple(address validator,uint256 signedBlocks)")

func (u *UptimeData) EncodeAbi() ([]byte, error) {
	return UptimeDataABIType.Encode(u)
}

func (u *UptimeData) DecodeAbi(buf []byte) error {
	return decodeStruct(UptimeDataABIType, buf, &u)
}

type Uptime struct {
	EpochID     *big.Int      `abi:"epochId"`
	UptimeData  []*UptimeData `abi:"uptimeData"`
	TotalBlocks *big.Int      `abi:"totalBlocks"`
}

var UptimeABIType = abi.MustNewType("tuple(uint256 epochId,tuple(address validator,uint256 signedBlocks)[] uptimeData,uint256 totalBlocks)")

func (u *Uptime) EncodeAbi() ([]byte, error) {
	return UptimeABIType.Encode(u)
}

func (u *Uptime) DecodeAbi(buf []byte) error {
	return decodeStruct(UptimeABIType, buf, &u)
}

type CommitEpochFunction struct {
	ID     *big.Int `abi:"id"`
	Epoch  *Epoch   `abi:"epoch"`
	Uptime *Uptime  `abi:"uptime"`
}

func (c *CommitEpochFunction) EncodeAbi() ([]byte, error) {
	return ChildValidatorSet.Abi.Methods["commitEpoch"].Encode(c)
}

func (c *CommitEpochFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildValidatorSet.Abi.Methods["commitEpoch"], buf, c)
}

type InitStruct struct {
	EpochReward   *big.Int `abi:"epochReward"`
	MinStake      *big.Int `abi:"minStake"`
	MinDelegation *big.Int `abi:"minDelegation"`
	EpochSize     *big.Int `abi:"epochSize"`
}

var InitStructABIType = abi.MustNewType("tuple(uint256 epochReward,uint256 minStake,uint256 minDelegation,uint256 epochSize)")

func (i *InitStruct) EncodeAbi() ([]byte, error) {
	return InitStructABIType.Encode(i)
}

func (i *InitStruct) DecodeAbi(buf []byte) error {
	return decodeStruct(InitStructABIType, buf, &i)
}

type ValidatorInit struct {
	Addr      types.Address `abi:"addr"`
	Pubkey    [4]*big.Int   `abi:"pubkey"`
	Signature [2]*big.Int   `abi:"signature"`
	Stake     *big.Int      `abi:"stake"`
}

var ValidatorInitABIType = abi.MustNewType("tuple(address addr,uint256[4] pubkey,uint256[2] signature,uint256 stake)")

func (v *ValidatorInit) EncodeAbi() ([]byte, error) {
	return ValidatorInitABIType.Encode(v)
}

func (v *ValidatorInit) DecodeAbi(buf []byte) error {
	return decodeStruct(ValidatorInitABIType, buf, &v)
}

type InitializeChildValidatorSetFunction struct {
	Init       *InitStruct      `abi:"init"`
	Validators []*ValidatorInit `abi:"validators"`
	NewBls     types.Address    `abi:"newBls"`
	Governance types.Address    `abi:"governance"`
}

func (i *InitializeChildValidatorSetFunction) EncodeAbi() ([]byte, error) {
	return ChildValidatorSet.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeChildValidatorSetFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildValidatorSet.Abi.Methods["initialize"], buf, i)
}

type AddToWhitelistFunction struct {
	WhitelistAddreses []ethgo.Address `abi:"whitelistAddreses"`
}

func (a *AddToWhitelistFunction) EncodeAbi() ([]byte, error) {
	return ChildValidatorSet.Abi.Methods["addToWhitelist"].Encode(a)
}

func (a *AddToWhitelistFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildValidatorSet.Abi.Methods["addToWhitelist"], buf, a)
}

type RegisterFunction struct {
	Signature [2]*big.Int `abi:"signature"`
	Pubkey    [4]*big.Int `abi:"pubkey"`
}

func (r *RegisterFunction) EncodeAbi() ([]byte, error) {
	return ChildValidatorSet.Abi.Methods["register"].Encode(r)
}

func (r *RegisterFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildValidatorSet.Abi.Methods["register"], buf, r)
}

type SyncStateFunction struct {
	Receiver types.Address `abi:"receiver"`
	Data     []byte        `abi:"data"`
}

func (s *SyncStateFunction) EncodeAbi() ([]byte, error) {
	return StateSender.Abi.Methods["syncState"].Encode(s)
}

func (s *SyncStateFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(StateSender.Abi.Methods["syncState"], buf, s)
}

type StateSyncedEvent struct {
	ID       *big.Int      `abi:"id"`
	Sender   types.Address `abi:"sender"`
	Receiver types.Address `abi:"receiver"`
	Data     []byte        `abi:"data"`
}

func (s *StateSyncedEvent) ParseLog(log *ethgo.Log) error {
	return decodeEvent(StateSender.Abi.Events["StateSynced"], log, s)
}

type L2StateSyncedEvent struct {
	ID       *big.Int      `abi:"id"`
	Sender   types.Address `abi:"sender"`
	Receiver types.Address `abi:"receiver"`
	Data     []byte        `abi:"data"`
}

func (l *L2StateSyncedEvent) ParseLog(log *ethgo.Log) error {
	return decodeEvent(L2StateSender.Abi.Events["L2StateSynced"], log, l)
}

type CheckpointMetadata struct {
	BlockHash               types.Hash `abi:"blockHash"`
	BlockRound              *big.Int   `abi:"blockRound"`
	CurrentValidatorSetHash types.Hash `abi:"currentValidatorSetHash"`
}

var CheckpointMetadataABIType = abi.MustNewType("tuple(bytes32 blockHash,uint256 blockRound,bytes32 currentValidatorSetHash)")

func (c *CheckpointMetadata) EncodeAbi() ([]byte, error) {
	return CheckpointMetadataABIType.Encode(c)
}

func (c *CheckpointMetadata) DecodeAbi(buf []byte) error {
	return decodeStruct(CheckpointMetadataABIType, buf, &c)
}

type Checkpoint struct {
	Epoch       *big.Int   `abi:"epoch"`
	BlockNumber *big.Int   `abi:"blockNumber"`
	EventRoot   types.Hash `abi:"eventRoot"`
}

var CheckpointABIType = abi.MustNewType("tuple(uint256 epoch,uint256 blockNumber,bytes32 eventRoot)")

func (c *Checkpoint) EncodeAbi() ([]byte, error) {
	return CheckpointABIType.Encode(c)
}

func (c *Checkpoint) DecodeAbi(buf []byte) error {
	return decodeStruct(CheckpointABIType, buf, &c)
}

type Validator struct {
	Address     types.Address `abi:"_address"`
	BlsKey      [4]*big.Int   `abi:"blsKey"`
	VotingPower *big.Int      `abi:"votingPower"`
}

var ValidatorABIType = abi.MustNewType("tuple(address _address,uint256[4] blsKey,uint256 votingPower)")

func (v *Validator) EncodeAbi() ([]byte, error) {
	return ValidatorABIType.Encode(v)
}

func (v *Validator) DecodeAbi(buf []byte) error {
	return decodeStruct(ValidatorABIType, buf, &v)
}

type SubmitFunction struct {
	CheckpointMetadata *CheckpointMetadata `abi:"checkpointMetadata"`
	Checkpoint         *Checkpoint         `abi:"checkpoint"`
	Signature          [2]*big.Int         `abi:"signature"`
	NewValidatorSet    []*Validator        `abi:"newValidatorSet"`
	Bitmap             []byte              `abi:"bitmap"`
}

func (s *SubmitFunction) EncodeAbi() ([]byte, error) {
	return CheckpointManager.Abi.Methods["submit"].Encode(s)
}

func (s *SubmitFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(CheckpointManager.Abi.Methods["submit"], buf, s)
}

type InitializeCheckpointManagerFunction struct {
	NewBls          types.Address `abi:"newBls"`
	NewBn256G2      types.Address `abi:"newBn256G2"`
	ChainID_        *big.Int      `abi:"chainId_"`
	NewValidatorSet []*Validator  `abi:"newValidatorSet"`
}

func (i *InitializeCheckpointManagerFunction) EncodeAbi() ([]byte, error) {
	return CheckpointManager.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeCheckpointManagerFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(CheckpointManager.Abi.Methods["initialize"], buf, i)
}

type ExitFunction struct {
	BlockNumber  *big.Int     `abi:"blockNumber"`
	LeafIndex    *big.Int     `abi:"leafIndex"`
	UnhashedLeaf []byte       `abi:"unhashedLeaf"`
	Proof        []types.Hash `abi:"proof"`
}

func (e *ExitFunction) EncodeAbi() ([]byte, error) {
	return ExitHelper.Abi.Methods["exit"].Encode(e)
}

func (e *ExitFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ExitHelper.Abi.Methods["exit"], buf, e)
}

type InitializeChildERC20PredicateFunction struct {
	NewL2StateSender          types.Address `abi:"newL2StateSender"`
	NewStateReceiver          types.Address `abi:"newStateReceiver"`
	NewRootERC20Predicate     types.Address `abi:"newRootERC20Predicate"`
	NewChildTokenTemplate     types.Address `abi:"newChildTokenTemplate"`
	NewNativeTokenRootAddress types.Address `abi:"newNativeTokenRootAddress"`
}

func (i *InitializeChildERC20PredicateFunction) EncodeAbi() ([]byte, error) {
	return ChildERC20Predicate.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeChildERC20PredicateFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildERC20Predicate.Abi.Methods["initialize"], buf, i)
}

type WithdrawToFunction struct {
	ChildToken types.Address `abi:"childToken"`
	Receiver   types.Address `abi:"receiver"`
	Amount     *big.Int      `abi:"amount"`
}

func (w *WithdrawToFunction) EncodeAbi() ([]byte, error) {
	return ChildERC20Predicate.Abi.Methods["withdrawTo"].Encode(w)
}

func (w *WithdrawToFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(ChildERC20Predicate.Abi.Methods["withdrawTo"], buf, w)
}

type InitializeNativeERC20Function struct {
	Predicate_ types.Address `abi:"predicate_"`
	RootToken_ types.Address `abi:"rootToken_"`
	Name_      string        `abi:"name_"`
	Symbol_    string        `abi:"symbol_"`
	Decimals_  uint8         `abi:"decimals_"`
}

func (i *InitializeNativeERC20Function) EncodeAbi() ([]byte, error) {
	return NativeERC20.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeNativeERC20Function) DecodeAbi(buf []byte) error {
	return decodeMethod(NativeERC20.Abi.Methods["initialize"], buf, i)
}

type InitializeRootERC20PredicateFunction struct {
	NewStateSender         types.Address `abi:"newStateSender"`
	NewExitHelper          types.Address `abi:"newExitHelper"`
	NewChildERC20Predicate types.Address `abi:"newChildERC20Predicate"`
	NewChildTokenTemplate  types.Address `abi:"newChildTokenTemplate"`
	NativeTokenRootAddress types.Address `abi:"nativeTokenRootAddress"`
}

func (i *InitializeRootERC20PredicateFunction) EncodeAbi() ([]byte, error) {
	return RootERC20Predicate.Abi.Methods["initialize"].Encode(i)
}

func (i *InitializeRootERC20PredicateFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(RootERC20Predicate.Abi.Methods["initialize"], buf, i)
}

type DepositToFunction struct {
	RootToken types.Address `abi:"rootToken"`
	Receiver  types.Address `abi:"receiver"`
	Amount    *big.Int      `abi:"amount"`
}

func (d *DepositToFunction) EncodeAbi() ([]byte, error) {
	return RootERC20Predicate.Abi.Methods["depositTo"].Encode(d)
}

func (d *DepositToFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(RootERC20Predicate.Abi.Methods["depositTo"], buf, d)
}

type ApproveFunction struct {
	Spender types.Address `abi:"spender"`
	Amount  *big.Int      `abi:"amount"`
}

func (a *ApproveFunction) EncodeAbi() ([]byte, error) {
	return RootERC20.Abi.Methods["approve"].Encode(a)
}

func (a *ApproveFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(RootERC20.Abi.Methods["approve"], buf, a)
}

type MintFunction struct {
	To     types.Address `abi:"to"`
	Amount *big.Int      `abi:"amount"`
}

func (m *MintFunction) EncodeAbi() ([]byte, error) {
	return RootERC20.Abi.Methods["mint"].Encode(m)
}

func (m *MintFunction) DecodeAbi(buf []byte) error {
	return decodeMethod(RootERC20.Abi.Methods["mint"], buf, m)
}
