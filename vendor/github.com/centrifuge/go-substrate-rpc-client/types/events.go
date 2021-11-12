// Go Substrate RPC Client (GSRPC) provides APIs and types around Polkadot and any Substrate-based chain RPC calls
//
// Copyright 2019 Centrifuge GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"fmt"

	"github.com/centrifuge/go-substrate-rpc-client/scale"
)

// EventBalancesEndowed is emitted when an account is created with some free balance
type EventBalancesEndowed struct {
	Phase   Phase
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventDustLost is emitted when an account is removed with a balance that is
// non-zero but below ExistentialDeposit, resulting in a loss.
type EventBalancesDustLost struct {
	Phase   Phase
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventBalancesTransfer is emitted when a transfer succeeded (from, to, value)
type EventBalancesTransfer struct {
	Phase  Phase
	From   AccountID
	To     AccountID
	Value  U128
	Topics []Hash
}

// EventBalanceSet is emitted when a balance is set by root
type EventBalancesBalanceSet struct {
	Phase    Phase
	Who      AccountID
	Free     U128
	Reserved U128
	Topics   []Hash
}

// EventDeposit is emitted when an account receives some free balance
type EventBalancesDeposit struct {
	Phase   Phase
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventBalancesReserved is emitted when some balance was reserved (moved from free to reserved)
type EventBalancesReserved struct {
	Phase   Phase
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventBalancesUnreserved is emitted when some balance was unreserved (moved from reserved to free)
type EventBalancesUnreserved struct {
	Phase   Phase
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventBalancesReserveRepatriated is emitted when some balance was moved from the reserve of the first account to the
// second account.
type EventBalancesReserveRepatriated struct {
	Phase             Phase
	From              AccountID
	To                AccountID
	Balance           U128
	DestinationStatus BalanceStatus
	Topics            []Hash
}

// EventGrandpaNewAuthorities is emitted when a new authority set has been applied
type EventGrandpaNewAuthorities struct {
	Phase          Phase
	NewAuthorities []struct {
		AuthorityID     AuthorityID
		AuthorityWeight U64
	}
	Topics []Hash
}

// EventGrandpaPaused is emitted when the current authority set has been paused
type EventGrandpaPaused struct {
	Phase  Phase
	Topics []Hash
}

// EventGrandpaResumed is emitted when the current authority set has been resumed
type EventGrandpaResumed struct {
	Phase  Phase
	Topics []Hash
}

// EventImOnlineHeartbeatReceived is emitted when a new heartbeat was received from AuthorityId
type EventImOnlineHeartbeatReceived struct {
	Phase       Phase
	AuthorityID AuthorityID
	Topics      []Hash
}

// EventImOnlineAllGood is emitted when at the end of the session, no offence was committed
type EventImOnlineAllGood struct {
	Phase  Phase
	Topics []Hash
}

// Exposure lists the own and nominated stake of a validator
type Exposure struct {
	Total  UCompact
	Own    UCompact
	Others []IndividualExposure
}

// IndividualExposure contains the nominated stake by one specific third party
type IndividualExposure struct {
	Who   AccountID
	Value UCompact
}

// EventImOnlineSomeOffline is emitted when the end of the session, at least once validator was found to be offline
type EventImOnlineSomeOffline struct {
	Phase                Phase
	IdentificationTuples []struct {
		ValidatorID        AccountID
		FullIdentification Exposure
	}
	Topics []Hash
}

// EventIndicesIndexAssigned is emitted when an index is assigned to an AccountID.
type EventIndicesIndexAssigned struct {
	Phase        Phase
	AccountID    AccountID
	AccountIndex AccountIndex
	Topics       []Hash
}

// EventIndicesIndexFreed is emitted when an index is unassigned.
type EventIndicesIndexFreed struct {
	Phase        Phase
	AccountIndex AccountIndex
	Topics       []Hash
}

// EventIndicesIndexFrozen is emitted when an index is frozen to its current account ID.
type EventIndicesIndexFrozen struct {
	Phase        Phase
	AccountIndex AccountIndex
	AccountID    AccountID
	Topics       []Hash
}

// EventOffencesOffence is emitted when there is an offence reported of the given kind happened at the session_index
// and (kind-specific) time slot. This event is not deposited for duplicate slashes
type EventOffencesOffence struct {
	Phase          Phase
	Kind           Bytes16
	OpaqueTimeSlot Bytes
	Topics         []Hash
}

// EventSessionNewSession is emitted when a new session has happened. Note that the argument is the session index,
// not the block number as the type might suggest
type EventSessionNewSession struct {
	Phase        Phase
	SessionIndex U32
	Topics       []Hash
}

// EventStakingEraPayout is emitted when the era payout has been set;
type EventStakingEraPayout struct {
	Phase           Phase
	EraIndex        U32
	ValidatorPayout U128
	Remainder       U128
	Topics          []Hash
}

// EventStakingReward is emitted when the staker has been rewarded by this amount.
type EventStakingReward struct {
	Phase  Phase
	Stash  AccountID
	Amount U128
	Topics []Hash
}

// EventStakingSlash is emitted when one validator (and its nominators) has been slashed by the given amount
type EventStakingSlash struct {
	Phase     Phase
	AccountID AccountID
	Balance   U128
	Topics    []Hash
}

// EventStakingOldSlashingReportDiscarded is emitted when an old slashing report from a prior era was discarded because
// it could not be processed
type EventStakingOldSlashingReportDiscarded struct {
	Phase        Phase
	SessionIndex U32
	Topics       []Hash
}

// EventStakingStakingElection is emitted when a new set of stakers was elected with the given
type EventStakingStakingElection struct {
	Phase   Phase
	Compute ElectionCompute
	Topics  []Hash
}

// EventStakingSolutionStored is emitted when a new solution for the upcoming election has been stored
type EventStakingSolutionStored struct {
	Phase   Phase
	Compute ElectionCompute
	Topics  []Hash
}

// EventStakingBonded is emitted when an account has bonded this amount
type EventStakingBonded struct {
	Phase  Phase
	Stash  AccountID
	Amount U128
	Topics []Hash
}

// EventStakingUnbonded is emitted when an account has unbonded this amount
type EventStakingUnbonded struct {
	Phase  Phase
	Stash  AccountID
	Amount U128
	Topics []Hash
}

// EventStakingWithdrawn is emitted when an account has called `withdraw_unbonded` and removed unbonding chunks
// worth `Balance` from the unlocking queue.
type EventStakingWithdrawn struct {
	Phase  Phase
	Stash  AccountID
	Amount U128
	Topics []Hash
}

// EventSystemExtrinsicSuccessV8 is emitted when an extrinsic completed successfully
//
// Deprecated: EventSystemExtrinsicSuccessV8 exists to allow users to simply implement their own EventRecords struct if
// they are on metadata version 8 or below. Use EventSystemExtrinsicSuccess otherwise
type EventSystemExtrinsicSuccessV8 struct {
	Phase  Phase
	Topics []Hash
}

// EventSystemExtrinsicSuccess is emitted when an extrinsic completed successfully
type EventSystemExtrinsicSuccess struct {
	Phase        Phase
	DispatchInfo DispatchInfo
	Topics       []Hash
}

// DispatchInfo contains a bundle of static information collected from the `#[weight = $x]` attributes.
type DispatchInfo struct {
	// Weight of this transaction
	Weight Weight
	// Class of this transaction
	Class DispatchClass
	// PaysFee indicates whether this transaction pays fees
	PaysFee bool
}

// DispatchClass is a generalized group of dispatch types. This is only distinguishing normal, user-triggered
// transactions (`Normal`) and anything beyond which serves a higher purpose to the system (`Operational`).
type DispatchClass struct {
	// A normal dispatch
	IsNormal bool
	// An operational dispatch
	IsOperational bool
}

func (d *DispatchClass) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	if b == 0 {
		d.IsNormal = true
	} else if b == 1 {
		d.IsOperational = true
	}
	return err
}

func (d DispatchClass) Encode(encoder scale.Encoder) error {
	var err error
	if d.IsNormal {
		err = encoder.PushByte(0)
	} else if d.IsOperational {
		err = encoder.PushByte(1)
	}
	return err
}

// EventSystemExtrinsicFailedV8 is emitted when an extrinsic failed
//
// Deprecated: EventSystemExtrinsicFailedV8 exists to allow users to simply implement their own EventRecords struct if
// they are on metadata version 8 or below. Use EventSystemExtrinsicFailed otherwise
type EventSystemExtrinsicFailedV8 struct {
	Phase         Phase
	DispatchError DispatchError
	Topics        []Hash
}

// EventSystemExtrinsicFailed is emitted when an extrinsic failed
type EventSystemExtrinsicFailed struct {
	Phase         Phase
	DispatchError DispatchError
	DispatchInfo  DispatchInfo
	Topics        []Hash
}

// EventSystemCodeUpdated is emitted when the runtime code (`:code`) is updated
type EventSystemCodeUpdated struct {
	Phase  Phase
	Topics []Hash
}

// EventSystemNewAccount is emitted when a new account was created
type EventSystemNewAccount struct {
	Phase  Phase
	Who    AccountID
	Topics []Hash
}

// EventSystemKilledAccount is emitted when an account is reaped
type EventSystemKilledAccount struct {
	Phase  Phase
	Who    AccountID
	Topics []Hash
}

// EventAssetIssued is emitted when an asset is issued.
type EventAssetIssued struct {
	Phase   Phase
	AssetID U32
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventAssetTransferred is emitted when an asset is transferred.
type EventAssetTransferred struct {
	Phase   Phase
	AssetID U32
	To      AccountID
	From    AccountID
	Balance U128
	Topics  []Hash
}

// EventAssetDestroyed is emitted when an asset is destroyed.
type EventAssetDestroyed struct {
	Phase   Phase
	AssetID U32
	Who     AccountID
	Balance U128
	Topics  []Hash
}

// EventDemocracyProposed is emitted when a motion has been proposed by a public account.
type EventDemocracyProposed struct {
	Phase         Phase
	ProposalIndex U32
	Balance       U128
	Topics        []Hash
}

// EventDemocracyTabled is emitted when a public proposal has been tabled for referendum vote.
type EventDemocracyTabled struct {
	Phase         Phase
	ProposalIndex U32
	Balance       U128
	Accounts      []AccountID
	Topics        []Hash
}

// EventDemocracyExternalTabled is emitted when an external proposal has been tabled.
type EventDemocracyExternalTabled struct {
	Phase  Phase
	Topics []Hash
}

// VoteThreshold is a means of determining if a vote is past pass threshold.
type VoteThreshold byte

const (
	// SuperMajorityApprove require super majority of approvals is needed to pass this vote.
	SuperMajorityApprove VoteThreshold = 0
	// SuperMajorityAgainst require super majority of rejects is needed to fail this vote.
	SuperMajorityAgainst VoteThreshold = 1
	// SimpleMajority require simple majority of approvals is needed to pass this vote.
	SimpleMajority VoteThreshold = 2
)

func (v *VoteThreshold) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	vb := VoteThreshold(b)
	switch vb {
	case SuperMajorityApprove, SuperMajorityAgainst, SimpleMajority:
		*v = vb
	default:
		return fmt.Errorf("unknown VoteThreshold enum: %v", vb)
	}
	return err
}

func (v VoteThreshold) Encode(encoder scale.Encoder) error {
	return encoder.PushByte(byte(v))
}

// EventDemocracyStarted is emitted when a referendum has begun.
type EventDemocracyStarted struct {
	Phase           Phase
	ReferendumIndex U32
	VoteThreshold   VoteThreshold
	Topics          []Hash
}

// EventDemocracyPassed is emitted when a proposal has been approved by referendum.
type EventDemocracyPassed struct {
	Phase           Phase
	ReferendumIndex U32
	Topics          []Hash
}

// EventDemocracyNotPassed is emitted when a proposal has been rejected by referendum.
type EventDemocracyNotPassed struct {
	Phase           Phase
	ReferendumIndex U32
	Topics          []Hash
}

// EventDemocracyCancelled is emitted when a referendum has been cancelled.
type EventDemocracyCancelled struct {
	Phase           Phase
	ReferendumIndex U32
	Topics          []Hash
}

// EventDemocracyExecuted is emitted when a proposal has been enacted.
type EventDemocracyExecuted struct {
	Phase           Phase
	ReferendumIndex U32
	Result          bool
	Topics          []Hash
}

// EventDemocracyDelegated is emitted when an account has delegated their vote to another account.
type EventDemocracyDelegated struct {
	Phase  Phase
	Who    AccountID
	Target AccountID
	Topics []Hash
}

// EventDemocracyUndelegated is emitted when an account has cancelled a previous delegation operation.
type EventDemocracyUndelegated struct {
	Phase  Phase
	Target AccountID
	Topics []Hash
}

// EventDemocracyVetoed is emitted when an external proposal has been vetoed.
type EventDemocracyVetoed struct {
	Phase       Phase
	Who         AccountID
	Hash        Hash
	BlockNumber BlockNumber
	Topics      []Hash
}

// EventDemocracyPreimageNoted is emitted when a proposal's preimage was noted, and the deposit taken.
type EventDemocracyPreimageNoted struct {
	Phase     Phase
	Hash      Hash
	AccountID AccountID
	Balance   U128
	Topics    []Hash
}

// EventDemocracyPreimageUsed is emitted when a proposal preimage was removed and used (the deposit was returned).
type EventDemocracyPreimageUsed struct {
	Phase     Phase
	Hash      Hash
	AccountID AccountID
	Balance   U128
	Topics    []Hash
}

// EventDemocracyPreimageInvalid is emitted when a proposal could not be executed because its preimage was invalid.
type EventDemocracyPreimageInvalid struct {
	Phase           Phase
	Hash            Hash
	ReferendumIndex U32
	Topics          []Hash
}

// EventDemocracyPreimageMissing is emitted when a proposal could not be executed because its preimage was missing.
type EventDemocracyPreimageMissing struct {
	Phase           Phase
	Hash            Hash
	ReferendumIndex U32
	Topics          []Hash
}

// EventDemocracyPreimageReaped is emitted when a registered preimage was removed
// and the deposit collected by the reaper (last item).
type EventDemocracyPreimageReaped struct {
	Phase    Phase
	Hash     Hash
	Provider AccountID
	Balance  U128
	Who      AccountID
	Topics   []Hash
}

// EventDemocracyUnlocked is emitted when an account has been unlocked successfully.
type EventDemocracyUnlocked struct {
	Phase     Phase
	AccountID AccountID
	Topics    []Hash
}

// EventCollectiveProposed is emitted when a motion (given hash) has been proposed (by given account)
// with a threshold (given `MemberCount`).
type EventCollectiveProposed struct {
	Phase         Phase
	Who           AccountID
	ProposalIndex U32
	Proposal      Hash
	MemberCount   U32
	Topics        []Hash
}

// EventCollectiveVote is emitted when a motion (given hash) has been voted on by given account, leaving
// a tally (yes votes and no votes given respectively as `MemberCount`).
type EventCollectiveVoted struct {
	Phase    Phase
	Who      AccountID
	Proposal Hash
	Approve  bool
	YesCount U32
	NoCount  U32
	Topics   []Hash
}

// EventCollectiveApproved is emitted when a motion was approved by the required threshold.
type EventCollectiveApproved struct {
	Phase    Phase
	Proposal Hash
	Topics   []Hash
}

// EventCollectiveDisapproved is emitted when a motion was not approved by the required threshold.
type EventCollectiveDisapproved struct {
	Phase    Phase
	Proposal Hash
	Topics   []Hash
}

// EventCollectiveExecuted is emitted when a motion was executed; `bool` is true if returned without error.
type EventCollectiveExecuted struct {
	Phase    Phase
	Proposal Hash
	Ok       bool
	Topics   []Hash
}

// EventCollectiveMemberExecuted is emitted when a single member did some action;
// `bool` is true if returned without error.
type EventCollectiveMemberExecuted struct {
	Phase    Phase
	Proposal Hash
	Ok       bool
	Topics   []Hash
}

// EventCollectiveClosed is emitted when a proposal was closed after its duration was up.
type EventCollectiveClosed struct {
	Phase    Phase
	Proposal Hash
	YesCount U32
	NoCount  U32
	Topics   []Hash
}

// EventTechnicalCommitteeProposed is emitted when a motion (given hash) has been proposed (by given account)
// with a threshold (given, `MemberCount`)
type EventTechnicalCommitteeProposed struct {
	Phase         Phase
	Account       AccountID
	ProposalIndex U32
	Proposal      Hash
	Threshold     U32
	Topics        []Hash
}

// EventTechnicalCommitteeVoted is emitted when a motion (given hash) has been voted on by given account, leaving,
// a tally (yes votes and no votes given respectively as `MemberCount`).
type EventTechnicalCommitteeVoted struct {
	Phase    Phase
	Account  AccountID
	Proposal Hash
	Voted    bool
	YesCount U32
	NoCount  U32
	Topics   []Hash
}

// EventTechnicalCommitteeApproved is emitted when a motion was approved by the required threshold.
type EventTechnicalCommitteeApproved struct {
	Phase    Phase
	Proposal Hash
	Topics   []Hash
}

// EventTechnicalCommitteeDisapproved is emitted when a motion was not approved by the required threshold.
type EventTechnicalCommitteeDisapproved struct {
	Phase    Phase
	Proposal Hash
	Topics   []Hash
}

// EventTechnicalCommitteeExecuted is emitted when a motion was executed;
// result will be `Ok` if it returned without error.
type EventTechnicalCommitteeExecuted struct {
	Phase    Phase
	Proposal Hash
	Result   DispatchResult
	Topics   []Hash
}

// EventTechnicalCommitteeMemberExecuted is emitted when a single member did some action;
// result will be `Ok` if it returned without error
type EventTechnicalCommitteeMemberExecuted struct {
	Phase    Phase
	Proposal Hash
	Result   DispatchResult
	Topics   []Hash
}

// EventTechnicalCommitteeClosed is emitted when A proposal was closed because its threshold was reached
// or after its duration was up
type EventTechnicalCommitteeClosed struct {
	Phase    Phase
	Proposal Hash
	YesCount U32
	NoCount  U32
	Topics   []Hash
}

// EventElectionsNewTerm is emitted when a new term with new members.
// This indicates that enough candidates existed, not that enough have has been elected.
// The inner value must be examined for this purpose.
type EventElectionsNewTerm struct {
	Phase      Phase
	NewMembers []struct {
		Member  AccountID
		Balance U128
	}
	Topics []Hash
}

// EventElectionsEmpty is emitted when No (or not enough) candidates existed for this round.
type EventElectionsEmptyTerm struct {
	Phase  Phase
	Topics []Hash
}

// EventElectionsMemberKicked is emitted when a member has been removed.
// This should always be followed by either `NewTerm` or `EmptyTerm`.
type EventElectionsMemberKicked struct {
	Phase  Phase
	Member AccountID
	Topics []Hash
}

// EventElectionsMemberRenounced is emitted when a member has renounced their candidacy.
type EventElectionsMemberRenounced struct {
	Phase  Phase
	Member AccountID
	Topics []Hash
}

// EventElectionsVoterReported is emitted when a voter (first element) was reported (by the second element)
// with the the report being successful or not (third element).
type EventElectionsVoterReported struct {
	Phase            Phase
	Target, Reporter AccountID
	Valid            bool
	Topics           []Hash
}

// A name was set or reset (which will remove all judgements).
type EventIdentitySet struct {
	Phase    Phase
	Identity AccountID
	Topics   []Hash
}

// A name was cleared, and the given balance returned.
type EventIdentityCleared struct {
	Phase    Phase
	Identity AccountID
	Balance  U128
	Topics   []Hash
}

// A name was removed and the given balance slashed.
type EventIdentityKilled struct {
	Phase    Phase
	Identity AccountID
	Balance  U128
	Topics   []Hash
}

// A judgement was asked from a registrar.
type EventIdentityJudgementRequested struct {
	Phase          Phase
	Sender         AccountID
	RegistrarIndex U32
	Topics         []Hash
}

// A judgement request was retracted.
type EventIdentityJudgementUnrequested struct {
	Phase          Phase
	Sender         AccountID
	RegistrarIndex U32
	Topics         []Hash
}

// A judgement was given by a registrar.
type EventIdentityJudgementGiven struct {
	Phase          Phase
	Target         AccountID
	RegistrarIndex U32
	Topics         []Hash
}

// A registrar was added.
type EventIdentityRegistrarAdded struct {
	Phase          Phase
	RegistrarIndex U32
	Topics         []Hash
}

// EventIdentitySubIdentityAdded is emitted when a sub-identity was added to an identity and the deposit paid
type EventIdentitySubIdentityAdded struct {
	Phase   Phase
	Sub     AccountID
	Main    AccountID
	Deposit U128
	Topics  []Hash
}

// EventIdentitySubIdentityRemoved is emitted when a sub-identity was removed from an identity and the deposit freed
type EventIdentitySubIdentityRemoved struct {
	Phase   Phase
	Sub     AccountID
	Main    AccountID
	Deposit U128
	Topics  []Hash
}

// EventIdentitySubIdentityRevoked is emitted when a sub-identity was cleared, and the given deposit repatriated from
// the main identity account to the sub-identity account.
type EventIdentitySubIdentityRevoked struct {
	Phase   Phase
	Sub     AccountID
	Main    AccountID
	Deposit U128
	Topics  []Hash
}

// EventSocietyFounded is emitted when the society is founded by the given identity
type EventSocietyFounded struct {
	Phase   Phase
	Founder AccountID
	Topics  []Hash
}

// EventSocietyBid is emitted when a membership bid just happened. The given account is the candidate's ID
// and their offer is the second
type EventSocietyBid struct {
	Phase     Phase
	Candidate AccountID
	Offer     U128
	Topics    []Hash
}

// EventSocietyVouch is emitted when a membership bid just happened by vouching.
// The given account is the candidate's ID and, their offer is the second. The vouching party is the third.
type EventSocietyVouch struct {
	Phase     Phase
	Candidate AccountID
	Offer     U128
	Vouching  AccountID
	Topics    []Hash
}

// EventSocietyAutoUnbid is emitted when a [candidate] was dropped (due to an excess of bids in the system)
type EventSocietyAutoUnbid struct {
	Phase     Phase
	Candidate AccountID
	Topics    []Hash
}

// EventSocietyUnbid is emitted when a [candidate] was dropped (by their request)
type EventSocietyUnbid struct {
	Phase     Phase
	Candidate AccountID
	Topics    []Hash
}

// EventSocietyUnvouch is emitted when a [candidate] was dropped (by request of who vouched for them)
type EventSocietyUnvouch struct {
	Phase     Phase
	Candidate AccountID
	Topics    []Hash
}

// EventSocietyInducted is emitted when a group of candidates have been inducted.
// The batch's primary is the first value, the batch in full is the second.
type EventSocietyInducted struct {
	Phase      Phase
	Primary    AccountID
	Candidates []AccountID
	Topics     []Hash
}

// EventSocietySuspendedMemberJudgement is emitted when a suspended member has been judged
type EventSocietySuspendedMemberJudgement struct {
	Phase  Phase
	Who    AccountID
	Judged bool
	Topics []Hash
}

// EventSocietyCandidateSuspended is emitted when a [candidate] has been suspended
type EventSocietyCandidateSuspended struct {
	Phase     Phase
	Candidate AccountID
	Topics    []Hash
}

// EventSocietyMemberSuspended is emitted when a [member] has been suspended
type EventSocietyMemberSuspended struct {
	Phase  Phase
	Member AccountID
	Topics []Hash
}

// EventSocietyChallenged is emitted when a [member] has been challenged
type EventSocietyChallenged struct {
	Phase  Phase
	Member AccountID
	Topics []Hash
}

// EventSocietyVote is emitted when a vote has been placed
type EventSocietyVote struct {
	Phase     Phase
	Candidate AccountID
	Voter     AccountID
	Vote      bool
	Topics    []Hash
}

// EventSocietyDefenderVote is emitted when a vote has been placed for a defending member
type EventSocietyDefenderVote struct {
	Phase  Phase
	Voter  AccountID
	Vote   bool
	Topics []Hash
}

// EventSocietyNewMaxMembers is emitted when a new [max] member count has been set
type EventSocietyNewMaxMembers struct {
	Phase  Phase
	Max    U32
	Topics []Hash
}

// EventSocietyUnfounded is emitted when society is unfounded
type EventSocietyUnfounded struct {
	Phase   Phase
	Founder AccountID
	Topics  []Hash
}

// EventSocietyDeposit is emitted when some funds were deposited into the society account
type EventSocietyDeposit struct {
	Phase  Phase
	Value  U128
	Topics []Hash
}

// EventRecoveryCreated is emitted when a recovery process has been set up for an account
type EventRecoveryCreated struct {
	Phase  Phase
	Who    AccountID
	Topics []Hash
}

// EventRecoveryInitiated is emitted when a recovery process has been initiated for account_1 by account_2
type EventRecoveryInitiated struct {
	Phase   Phase
	Account AccountID
	Who     AccountID
	Topics  []Hash
}

// EventRecoveryVouched is emitted when a recovery process for account_1 by account_2 has been vouched for by account_3
type EventRecoveryVouched struct {
	Phase   Phase
	Lost    AccountID
	Rescuer AccountID
	Who     AccountID
	Topics  []Hash
}

// EventRecoveryClosed is emitted when a recovery process for account_1 by account_2 has been closed
type EventRecoveryClosed struct {
	Phase   Phase
	Who     AccountID
	Rescuer AccountID
	Topics  []Hash
}

// EventRecoveryAccountRecovered is emitted when account_1 has been successfully recovered by account_2
type EventRecoveryAccountRecovered struct {
	Phase   Phase
	Who     AccountID
	Rescuer AccountID
	Topics  []Hash
}

// EventRecoveryRemoved is emitted when a recovery process has been removed for an account
type EventRecoveryRemoved struct {
	Phase  Phase
	Who    AccountID
	Topics []Hash
}

// EventVestingVestingUpdated is emitted when the amount vested has been updated.
// This could indicate more funds are available.
// The balance given is the amount which is left unvested (and thus locked)
type EventVestingVestingUpdated struct {
	Phase    Phase
	Account  AccountID
	Unvested U128
	Topics   []Hash
}

// EventVestingVestingCompleted is emitted when an [account] has become fully vested. No further vesting can happen
type EventVestingVestingCompleted struct {
	Phase   Phase
	Account AccountID
	Topics  []Hash
}

// EventSchedulerScheduled is emitted when scheduled some task
type EventSchedulerScheduled struct {
	Phase  Phase
	When   BlockNumber
	Index  U32
	Topics []Hash
}

// EventSchedulerCanceled is emitted when canceled some task
type EventSchedulerCanceled struct {
	Phase  Phase
	When   BlockNumber
	Index  U32
	Topics []Hash
}

// EventSchedulerDispatched is emitted when dispatched some task
type EventSchedulerDispatched struct {
	Phase  Phase
	Task   TaskAddress
	ID     OptionBytes
	Result DispatchResult
	Topics []Hash
}

type ProxyType byte

const (
	Any         ProxyType = 0
	NonTransfer ProxyType = 1
	Governance  ProxyType = 2
	Staking     ProxyType = 3
)

func (pt *ProxyType) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	vb := ProxyType(b)
	switch vb {
	case Any, NonTransfer, Governance, Staking:
		*pt = vb
	default:
		return fmt.Errorf("unknown ProxyType enum: %v", vb)
	}
	return err
}

func (pt ProxyType) Encode(encoder scale.Encoder) error {
	return encoder.PushByte(byte(pt))
}

// EventProxyProxyExecuted is emitted when a proxy was executed correctly, with the given [result]
type EventProxyProxyExecuted struct {
	Phase  Phase
	Result DispatchResult
	Topics []Hash
}

// EventProxyAnonymousCreated is emitted when an anonymous account has been created by new proxy with given,
// disambiguation index and proxy type.
type EventProxyAnonymousCreated struct {
	Phase               Phase
	Anonymous           AccountID
	Who                 AccountID
	ProxyType           ProxyType
	DisambiguationIndex U16
	Topics              []Hash
}

// EventSudoSudid is emitted when a sudo just took place.
type EventSudoSudid struct {
	Phase  Phase
	Result DispatchResult
	Topics []Hash
}

// EventSudoKeyChanged is emitted when the sudoer just switched identity; the old key is supplied.
type EventSudoKeyChanged struct {
	Phase     Phase
	AccountID AccountID
	Topics    []Hash
}

// A sudo just took place.
type EventSudoAsDone struct {
	Phase  Phase
	Done   bool
	Topics []Hash
}

// EventTreasuryProposed is emitted when New proposal.
type EventTreasuryProposed struct {
	Phase         Phase
	ProposalIndex U32
	Topics        []Hash
}

// EventTreasurySpending is emitted when we have ended a spend period and will now allocate funds.
type EventTreasurySpending struct {
	Phase           Phase
	BudgetRemaining U128
	Topics          []Hash
}

// EventTreasuryAwarded is emitted when some funds have been allocated.
type EventTreasuryAwarded struct {
	Phase         Phase
	ProposalIndex U32
	Amount        U128
	Beneficiary   AccountID
	Topics        []Hash
}

// EventTreasuryRejected is emitted when s proposal was rejected; funds were slashed.
type EventTreasuryRejected struct {
	Phase         Phase
	ProposalIndex U32
	Amount        U128
	Topics        []Hash
}

// EventTreasuryBurnt is emitted when some of our funds have been burnt.
type EventTreasuryBurnt struct {
	Phase  Phase
	Burn   U128
	Topics []Hash
}

// EventTreasuryRollover is emitted when spending has finished; this is the amount that rolls over until next spend.
type EventTreasuryRollover struct {
	Phase           Phase
	BudgetRemaining U128
	Topics          []Hash
}

// EventTreasuryDeposit is emitted when some funds have been deposited.
type EventTreasuryDeposit struct {
	Phase     Phase
	Deposited U128
	Topics    []Hash
}

// EventTreasuryNewTip is emitted when a new tip suggestion has been opened.
type EventTreasuryNewTip struct {
	Phase  Phase
	Hash   Hash
	Topics []Hash
}

// EventTreasuryTipClosing is emitted when a tip suggestion has reached threshold and is closing.
type EventTreasuryTipClosing struct {
	Phase  Phase
	Hash   Hash
	Topics []Hash
}

// EventTreasuryTipClosed is emitted when a tip suggestion has been closed.
type EventTreasuryTipClosed struct {
	Phase     Phase
	Hash      Hash
	AccountID AccountID
	Balance   U128
	Topics    []Hash
}

// EventTreasuryTipRetracted is emitted when a tip suggestion has been retracted.
type EventTreasuryTipRetracted struct {
	Phase  Phase
	Hash   Hash
	Topics []Hash
}

// EventContractsInstantiated is emitted when a contract is deployed by address at the specified address
type EventContractsInstantiated struct {
	Phase    Phase
	Owner    AccountID
	Contract AccountID
	Topics   []Hash
}

// EventContractsEvicted is emitted when a contract has been evicted and is now in tombstone state.
type EventContractsEvicted struct {
	Phase     Phase
	Contract  AccountID
	Tombstone bool
	Topics    []Hash
}

// EventContractsRestored is emitted when a restoration for a contract has been successful.
type EventContractsRestored struct {
	Phase         Phase
	Donor         AccountID
	Destination   AccountID
	CodeHash      Hash
	RentAllowance U128
	Topics        []Hash
}

// EventContractsCodeStored is emitted when code with the specified hash has been stored
type EventContractsCodeStored struct {
	Phase    Phase
	CodeHash Hash
	Topics   []Hash
}

// EventContractsScheduleUpdated is triggered when the current [schedule] is updated
type EventContractsScheduleUpdated struct {
	Phase    Phase
	Schedule U32
	Topics   []Hash
}

// EventContractsContractExecution is triggered when an event deposited upon execution of a contract from the account
type EventContractsContractExecution struct {
	Phase   Phase
	Account AccountID
	Data    Bytes
	Topics  []Hash
}

// EventUtilityBatchInterrupted is emitted when a batch of dispatches did not complete fully.
//Index of first failing dispatch given, as well as the error.
type EventUtilityBatchInterrupted struct {
	Phase         Phase
	Index         U32
	DispatchError DispatchError
	Topics        []Hash
}

// EventUtilityBatchCompleted is emitted when a batch of dispatches completed fully with no error.
type EventUtilityBatchCompleted struct {
	Phase  Phase
	Topics []Hash
}

// EventUtilityNewMultisig is emitted when a new multisig operation has begun.
// First param is the account that is approving, second is the multisig account, third is hash of the call.
type EventMultisigNewMultisig struct {
	Phase    Phase
	Who, ID  AccountID
	CallHash Hash
	Topics   []Hash
}

// TimePoint is a global extrinsic index, formed as the extrinsic index within a block,
// together with that block's height.
type TimePoint struct {
	Height U32
	Index  U32
}

// TaskAddress holds the location of a scheduled task that can be used to remove it
type TaskAddress struct {
	When  BlockNumber
	Index U32
}

// EventUtility is emitted when a multisig operation has been approved by someone. First param is the account that is
// approving, third is the multisig account, fourth is hash of the call.
type EventMultisigApproval struct {
	Phase     Phase
	Who       AccountID
	TimePoint TimePoint
	ID        AccountID
	CallHash  Hash
	Topics    []Hash
}

// DispatchResult can be returned from dispatchable functions
type DispatchResult struct {
	Ok    bool
	Error DispatchError
}

func (d *DispatchResult) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	if err != nil {
		return err
	}

	switch b {
	case 0:
		d.Ok = true
		return nil
	default:
		derr := DispatchError{}
		err = decoder.Decode(&derr)
		if err != nil {
			return err
		}
		d.Error = derr
		return nil
	}
}

func (d DispatchResult) Encode(encoder scale.Encoder) error {
	if d.Ok {
		return encoder.PushByte(0)
	}
	return d.Error.Encode(encoder)
}

// EventUtility is emitted when a multisig operation has been executed. First param is the account that is
// approving, third is the multisig account, fourth is hash of the call to be executed.
type EventMultisigExecuted struct {
	Phase     Phase
	Who       AccountID
	TimePoint TimePoint
	ID        AccountID
	CallHash  Hash
	Result    DispatchResult
	Topics    []Hash
}

// EventUtility is emitted when a multisig operation has been cancelled. First param is the account that is
// cancelling, third is the multisig account, fourth is hash of the call.
type EventMultisigCancelled struct {
	Phase     Phase
	Who       AccountID
	TimePoint TimePoint
	ID        AccountID
	CallHash  Hash
	Topics    []Hash
}
