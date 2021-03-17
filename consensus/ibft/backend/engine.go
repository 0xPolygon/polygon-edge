package backend

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"math/rand"
	"time"

	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/consensus/ibft/core"
	"github.com/0xPolygon/minimal/consensus/ibft/validator"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

const (
	checkpointInterval = 1024 // Number of blocks after which to save the vote snapshot to the database
	inmemorySnapshots  = 128  // Number of recent vote snapshots to keep in memory
	inmemoryPeers      = 40
	inmemoryMessages   = 1024
)

var (
	// errInvalidProposal is returned when a prposal is malformed.
	errInvalidProposal = errors.New("invalid proposal")
	// errInvalidSignature is returned when given signature is not signed by given
	// address.
	errInvalidSignature = errors.New("invalid signature")
	// errUnknownBlock is returned when the list of validators is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errUnauthorized is returned if a header is signed by a non authorized entity.
	errUnauthorized = errors.New("unauthorized")
	// errInvalidDifficulty is returned if the difficulty of a block is not 1
	errInvalidDifficulty = errors.New("invalid difficulty")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errInvalidMixDigest is returned if a block's mix digest is not Istanbul digest.
	errInvalidMixDigest = errors.New("invalid Istanbul mix digest")
	// errInvalidNonce is returned if a block's nonce is invalid
	errInvalidNonce = errors.New("invalid nonce")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")
	// errInconsistentValidatorSet is returned if the validator set is inconsistent
	errInconsistentValidatorSet = errors.New("non empty uncle hash")
	// errInvalidTimestamp is returned if the timestamp of a block is lower than the previous block's timestamp + the minimum block period.
	errInvalidTimestamp = errors.New("invalid timestamp")
	// errInvalidVotingChain is returned if an authorization list is attempted to
	// be modified via out-of-range or non-contiguous headers.
	errInvalidVotingChain = errors.New("invalid voting chain")
	// errInvalidVote is returned if a nonce value is something else that the two
	// allowed constants of 0x00..0 or 0xff..f.
	errInvalidVote = errors.New("vote nonce not 0x00..0 or 0xff..f")
	// errInvalidCommittedSeals is returned if the committed seal is not signed by any of parent validators.
	errInvalidCommittedSeals = errors.New("invalid committed seals")
	// errEmptyCommittedSeals is returned if the field of committed seals is zero.
	errEmptyCommittedSeals = errors.New("zero committed seals")
	// errMismatchTxhashes is returned if the TxHash in header is mismatch.
	errMismatchTxhashes = errors.New("mismatch transactions hashes")
)
var (
	defaultDifficulty = big.NewInt(1)
	nilUncleHash      = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	emptyNonce        = types.Nonce{}
	now               = time.Now

	nonceAuthVote = hex.MustDecodeHex("0xffffffffffffffff") // Magic nonce number to vote on adding a new validator
	nonceDropVote = hex.MustDecodeHex("0x0000000000000000") // Magic nonce number to vote on removing a validator.

	inmemoryAddresses  = 20 // Number of recent addresses from ecrecover
	recentAddresses, _ = lru.NewARC(inmemoryAddresses)
)

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *backend) Author(header *types.Header) (types.Address, error) {
	return ecrecover(header)
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
//func (sb *backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
//	return sb.verifyHeader(chain, header, nil)
//}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *backend) verifyHeader(header *types.Header, parents []*types.Header) error {
	// Don't waste time checking blocks from the future
	if new(big.Int).SetUint64(header.Timestamp).Cmp(big.NewInt(now().Unix())) > 0 {
		return consensus.ErrFutureBlock
	}

	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractIstanbulExtra(header); err != nil {
		return errInvalidExtraDataFormat
	}

	// Ensure that the coinbase is valid
	if header.Nonce != (emptyNonce) && !bytes.Equal(header.Nonce[:], nonceAuthVote) && !bytes.Equal(header.Nonce[:], nonceDropVote) {
		return errInvalidNonce
	}
	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixHash != types.IstanbulDigest {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in Istanbul
	if header.Sha3Uncles != nilUncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if new(big.Int).SetUint64(header.Difficulty).Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	return sb.verifyCascadingFields(header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *backend) verifyCascadingFields(header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number
	if number == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent, _ = sb.blockchain.GetHeaderByHash(header.ParentHash)
	}
	if parent == nil || parent.Number != number-1 || parent.Hash != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if parent.Timestamp+sb.config.BlockPeriod > header.Timestamp {
		return errInvalidTimestamp
	}
	// Verify validators in extraData. Validators in snapshot and extraData should be the same.
	snap, err := sb.snapshot(number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}
	validators := make([]byte, len(snap.validators())*types.AddressLength)
	for i, validator := range snap.validators() {
		copy(validators[i*types.AddressLength:], validator[:])
	}
	if err := sb.verifySigner(header, parents); err != nil {
		return err
	}

	return sb.verifyCommittedSeals(header, parents)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *backend) VerifyHeaders(headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *backend) VerifyUncles(block *types.Block) error {
	if len(block.Uncles) > 0 {
		return errInvalidUncleHash
	}
	return nil
}

// verifySigner checks whether the signer is in parent's validator set
func (sb *backend) verifySigner(header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number
	if number == 0 {
		return errUnknownBlock
	}

	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := sb.snapshot(number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// resolve the authorization key and check against signers
	signer, err := ecrecover(header)
	if err != nil {
		return err
	}

	// Signer should be in the validator set of previous block's extraData.
	if _, v := snap.ValSet.GetByAddress(signer); v == nil {
		return errUnauthorized
	}
	return nil
}

// verifyCommittedSeals checks whether every committed seal is signed by one of the parent's validators
func (sb *backend) verifyCommittedSeals(header *types.Header, parents []*types.Header) error {
	number := header.Number
	// We don't need to verify committed seals in the genesis block
	if number == 0 {
		return nil
	}

	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := sb.snapshot(number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return err
	}
	// The length of Committed seals should be larger than 0
	if len(extra.CommittedSeal) == 0 {
		return errEmptyCommittedSeals
	}

	validators := snap.ValSet.Copy()
	// Check whether the committed seals are generated by parent's validators
	validSeal := 0
	proposalSeal := core.PrepareCommittedSeal(header.Hash)
	// 1. Get committed seals from current header
	for _, seal := range extra.CommittedSeal {
		// 2. Get the original address by seal and parent block hash
		addr, err := ibft.GetSignatureAddress(proposalSeal, seal)
		if err != nil {
			return errInvalidSignature
		}
		// Every validator can have only one seal. If more than one seals are signed by a
		// validator, the validator cannot be found and errInvalidCommittedSeals is returned.
		if validators.RemoveValidator(addr) {
			validSeal += 1
		} else {
			return errInvalidCommittedSeals
		}
	}

	// The length of validSeal should be larger than number of faulty node + 1
	if validSeal <= 2*snap.ValSet.F() {
		return errInvalidCommittedSeals
	}

	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *backend) VerifySeal(header *types.Header) error {
	// get parent header and ensure the signer is in parent's validator set
	number := header.Number
	if number == 0 {
		return errUnknownBlock
	}

	// ensure that the difficulty equals to defaultDifficulty
	if new(big.Int).SetUint64(header.Difficulty).Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}
	return sb.verifySigner(header, nil)
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *backend) Prepare(header *types.Header) error {
	// unused fields, force to set to empty
	header.Miner = types.Address{}
	header.Nonce = emptyNonce
	header.MixHash = types.IstanbulDigest

	// copy the parent extra data as the header extra data
	number := header.Number
	parent, _ := sb.blockchain.GetHeaderByHash(header.ParentHash)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// use the same difficulty for all blocks
	header.Difficulty = defaultDifficulty.Uint64()

	// Assemble the voting snapshot
	snap, err := sb.snapshot(number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	// get valid candidate list
	sb.candidatesLock.RLock()
	var addresses []types.Address
	var authorizes []bool
	for address, authorize := range sb.candidates {
		if snap.checkVote(address, authorize) {
			addresses = append(addresses, address)
			authorizes = append(authorizes, authorize)
		}
	}
	sb.candidatesLock.RUnlock()

	// pick one of the candidates randomly
	if len(addresses) > 0 {
		index := rand.Intn(len(addresses))
		// add validator voting in coinbase
		header.Miner = addresses[index]
		if authorizes[index] {
			copy(header.Nonce[:], nonceAuthVote)
		} else {
			copy(header.Nonce[:], nonceDropVote)
		}
	}

	// add validators in snapshot to extraData's validators section
	extra, err := prepareExtra(header, snap.validators())
	if err != nil {
		return err
	}
	header.ExtraData = extra

	// set header's timestamp
	header.Timestamp = new(big.Int).Add(new(big.Int).SetUint64(parent.Timestamp), new(big.Int).SetUint64(sb.config.BlockPeriod)).Uint64()
	if int64(header.Timestamp) < time.Now().Unix() {
		header.Timestamp = uint64(time.Now().Unix())
	}
	return nil
}

/*
// TODO: Remove
// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *backend) Finalize(header *types.Header, state state.Snapshot, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	// No block rewards in Istanbul, so the state remains as is and uncles are dropped
	parent, _ := chain.CurrentHeader()
	transition, _ := chain.Executor().BeginTxn(parent.Hash, header)
	for _, tx := range txs {
		transition.Write(tx)
	}

	_, root := transition.Commit()
	header.StateRoot = root
	header.Sha3Uncles = nilUncleHash

	// Assemble and return the final block for sealing
	return &types.Block{
		Header:       header,
		Transactions: txs,
		Uncles:       nil,
	}, nil
}
*/

/*
// TODO: Remove
// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (sb *backend) CalcDifficulty(time uint64, parent *types.Header) *big.Int {
	return defaultDifficulty
}
*/

// update timestamp and signature of the block based on its number of transactions
func (sb *backend) updateBlock(parent *types.Header, block *types.Block) (*types.Block, error) {
	header := block.Header
	// sign the hash
	seal, err := sb.Sign(sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}

	err = writeSeal(header, seal)
	if err != nil {
		return nil, err
	}

	return block.WithSeal(header), nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
//func (sb *backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
//	return sb.verifyHeader(chain, header, nil)
//}

func (sb *backend) VerifyHeader(parent, header *types.Header, uncle, seal bool) error {
	return sb.verifyHeader(header, nil)
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *backend) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	// update the block header timestamp and signature and propose the block to core engine
	header := block.Header
	number := header.Number

	// Bail out if we're unauthorized to sign a block
	snap, err := sb.snapshot(number-1, header.ParentHash, nil)
	if err != nil {
		return nil, err
	}
	if _, v := snap.ValSet.GetByAddress(sb.address); v == nil {
		return nil, errUnauthorized
	}

	parent, ok := sb.blockchain.GetHeaderByHash(header.ParentHash)
	if !ok {
		return nil, consensus.ErrUnknownAncestor
	}
	block, err = sb.updateBlock(parent, block)
	if err != nil {
		return nil, err
	}

	// wait for the timestamp of header, use this to adjust the block period
	delay := time.Unix(int64(block.Header.Timestamp), 0).Sub(now())
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, nil
	}

	// get the proposed block hash and clear it if the seal() is completed.
	sb.sealMu.Lock()
	sb.proposedBlockHash = block.Hash()
	clear := func() {
		sb.proposedBlockHash = types.Hash{}
		sb.sealMu.Unlock()
	}
	defer clear()

	// post block into Istanbul engine
	go sb.EventMux().Post(ibft.RequestEvent{
		Proposal: block,
	})

	for {
		select {
		case result := <-sb.commitCh:
			// if the block hash and the hash from channel are the same,
			// return the result. Otherwise, keep waiting the next hash.
			if block.Hash() == result.Hash() {
				return result, nil
			}
		case <-ctx.Done():
			return nil, nil
		}
	}
}

func (sb *backend) Close() error {
	return nil
}

// Start implements consensus.Istanbul.Start
func (sb *backend) Start() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return ibft.ErrStartedEngine
	}

	// clear previous data
	sb.proposedBlockHash = types.Hash{}
	if sb.commitCh != nil {
		close(sb.commitCh)
	}
	sb.commitCh = make(chan *types.Block, 1)

	//sb.currentBlock = currentBlock
	//sb.hasBadBlock = hasBadBlock

	if err := sb.core.Start(); err != nil {
		return err
	}

	sb.coreStarted = true
	return nil
}

// Stop implements consensus.Istanbul.Stop
func (sb *backend) Stop() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return ibft.ErrStoppedEngine
	}
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted = false
	return nil
}

// snapshot retrieves the authorization snapshot at a given point in time.
func (sb *backend) snapshot(number uint64, hash types.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk for checkpoints
	var (
		headers []*types.Header
		snap    *Snapshot
	)
	for snap == nil {
		// If an in-memory snapshot was found, use that
		if s, ok := sb.recents.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}
		// If an on-disk checkpoint snapshot can be found, use that
		if number%checkpointInterval == 0 {
			if s, err := loadSnapshot(sb.config.Epoch, sb.db, hash); err == nil {
				snap = s
				break
			}
		}
		// If we're at block zero, make a snapshot
		if number == 0 {
			genesis, _ := sb.blockchain.GetHeaderByHash(sb.blockchain.Genesis())
			if err := sb.VerifyHeader(nil, genesis, false, false); err != nil {
				return nil, err
			}
			istanbulExtra, err := types.ExtractIstanbulExtra(genesis)
			if err != nil {
				return nil, err
			}
			snap = newSnapshot(sb.config.Epoch, 0, genesis.Hash, validator.NewSet(istanbulExtra.Validators, sb.config.ProposerPolicy))
			if err := snap.store(sb.db); err != nil {
				return nil, err
			}
			break
		}
		// No snapshot for this header, gather the header and move backward
		var header *types.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			header = parents[len(parents)-1]
			if header.Hash != hash || header.Number != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			header, _ = sb.blockchain.GetHeaderByHash(hash)
			if header == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}
		headers = append(headers, header)
		number, hash = number-1, header.ParentHash
	}
	// Previous snapshot found, apply any pending headers on top of it
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}
	snap, err := snap.apply(headers)
	if err != nil {
		return nil, err
	}
	sb.recents.Add(snap.Hash, snap)

	// If we've generated a new checkpoint snapshot, save to disk
	if snap.Number%checkpointInterval == 0 && len(headers) > 0 {
		if err = snap.store(sb.db); err != nil {
			return nil, err
		}
	}
	return snap, err
}

// FIXME: Need to update this for Istanbul
// sigHash returns the hash which is used as input for the Istanbul
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash types.Hash) {
	hasher := keccak.NewKeccak256()

	// Clean seal is required for calculating proposer seal.
	rlp.Encode(hasher, types.IstanbulFilteredHeader(header, false))
	hasher.Sum(hash[:0])
	return hash
}

// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header) (types.Address, error) {
	hash := header.Hash
	if addr, ok := recentAddresses.Get(hash); ok {
		return addr.(types.Address), nil
	}

	// Retrieve the signature from the header extra-data
	istanbulExtra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return types.Address{}, err
	}

	addr, err := ibft.GetSignatureAddress(sigHash(header).Bytes(), istanbulExtra.Seal)
	if err != nil {
		return addr, err
	}
	recentAddresses.Add(hash, addr)
	return addr, nil
}

// prepareExtra returns a extra-data of the given header and validators
func prepareExtra(header *types.Header, vals []types.Address) ([]byte, error) {
	var buf bytes.Buffer

	// compensate the lack bytes if header.Extra is not enough IstanbulExtraVanity bytes.
	if len(header.ExtraData) < types.IstanbulExtraVanity {
		header.ExtraData = append(header.ExtraData, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(header.ExtraData))...)
	}
	buf.Write(header.ExtraData[:types.IstanbulExtraVanity])

	ist := &types.IstanbulExtra{
		Validators:    vals,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	payload, err := rlp.EncodeToBytes(&ist)
	if err != nil {
		return nil, err
	}

	return append(buf.Bytes(), payload...), nil
}

// writeSeal writes the extra-data field of the given header with the given seals.
// suggest to rename to writeSeal.
func writeSeal(h *types.Header, seal []byte) error {
	if len(seal)%types.IstanbulExtraSeal != 0 {
		return errInvalidSignature
	}

	istanbulExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		return err
	}

	istanbulExtra.Seal = seal
	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return err
	}

	h.ExtraData = append(h.ExtraData[:types.IstanbulExtraVanity], payload...)
	return nil
}

// writeCommittedSeals writes the extra-data field of a block header with given committed seals.
func writeCommittedSeals(h *types.Header, committedSeals [][]byte) error {
	if len(committedSeals) == 0 {
		return errInvalidCommittedSeals
	}

	for _, seal := range committedSeals {
		if len(seal) != types.IstanbulExtraSeal {
			return errInvalidCommittedSeals
		}
	}

	istanbulExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		return err
	}

	istanbulExtra.CommittedSeal = make([][]byte, len(committedSeals))
	copy(istanbulExtra.CommittedSeal, committedSeals)

	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return err
	}

	h.ExtraData = append(h.ExtraData[:types.IstanbulExtraVanity], payload...)
	return nil
}
