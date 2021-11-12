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
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/centrifuge/go-substrate-rpc-client/scale"
	"github.com/ethereum/go-ethereum/log"
)

// EventRecordsRaw is a raw record for a set of events, represented as the raw bytes. It exists since
// decoding of events can only be done with metadata, so events can't follow the static way of decoding
// other types do. It exposes functions to decode events using metadata and targets.
// Be careful using this in your own structs – it only works as the last value in a struct since it will consume the
// remainder of the encoded data. The reason for this is that it does not contain any length encoding, so it would
// not know where to stop.
type EventRecordsRaw []byte

// Encode implements encoding for Data, which just unwraps the bytes of Data
func (e EventRecordsRaw) Encode(encoder scale.Encoder) error {
	return encoder.Write(e)
}

// Decode implements decoding for Data, which just reads all the remaining bytes into Data
func (e *EventRecordsRaw) Decode(decoder scale.Decoder) error {
	for i := 0; true; i++ {
		b, err := decoder.ReadOneByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		*e = append((*e)[:i], b)
	}
	return nil
}

// EventRecords is a default set of possible event records that can be used as a target for
// `func (e EventRecordsRaw) Decode(...`
type EventRecords struct {
	Balances_Endowed                   []EventBalancesEndowed                   //nolint:stylecheck,golint
	Balances_DustLost                  []EventBalancesDustLost                  //nolint:stylecheck,golint
	Balances_Transfer                  []EventBalancesTransfer                  //nolint:stylecheck,golint
	Balances_BalanceSet                []EventBalancesBalanceSet                //nolint:stylecheck,golint
	Balances_Deposit                   []EventBalancesDeposit                   //nolint:stylecheck,golint
	Balances_Reserved                  []EventBalancesReserved                  //nolint:stylecheck,golint
	Balances_Unreserved                []EventBalancesUnreserved                //nolint:stylecheck,golint
	Balances_ReservedRepatriated       []EventBalancesReserveRepatriated        //nolint:stylecheck,golint
	Grandpa_NewAuthorities             []EventGrandpaNewAuthorities             //nolint:stylecheck,golint
	Grandpa_Paused                     []EventGrandpaPaused                     //nolint:stylecheck,golint
	Grandpa_Resumed                    []EventGrandpaResumed                    //nolint:stylecheck,golint
	ImOnline_HeartbeatReceived         []EventImOnlineHeartbeatReceived         //nolint:stylecheck,golint
	ImOnline_AllGood                   []EventImOnlineAllGood                   //nolint:stylecheck,golint
	ImOnline_SomeOffline               []EventImOnlineSomeOffline               //nolint:stylecheck,golint
	Indices_IndexAssigned              []EventIndicesIndexAssigned              //nolint:stylecheck,golint
	Indices_IndexFreed                 []EventIndicesIndexFreed                 //nolint:stylecheck,golint
	Indices_IndexFrozen                []EventIndicesIndexFrozen                //nolint:stylecheck,golint
	Offences_Offence                   []EventOffencesOffence                   //nolint:stylecheck,golint
	Session_NewSession                 []EventSessionNewSession                 //nolint:stylecheck,golint
	Staking_EraPayout                  []EventStakingEraPayout                  //nolint:stylecheck,golint
	Staking_Reward                     []EventStakingReward                     //nolint:stylecheck,golint
	Staking_Slash                      []EventStakingSlash                      //nolint:stylecheck,golint
	Staking_OldSlashingReportDiscarded []EventStakingOldSlashingReportDiscarded //nolint:stylecheck,golint
	Staking_StakingElection            []EventStakingStakingElection            //nolint:stylecheck,golint
	Staking_SolutionStored             []EventStakingSolutionStored             //nolint:stylecheck,golint
	Staking_Bonded                     []EventStakingBonded                     //nolint:stylecheck,golint
	Staking_Unbonded                   []EventStakingUnbonded                   //nolint:stylecheck,golint
	Staking_Withdrawn                  []EventStakingWithdrawn                  //nolint:stylecheck,golint
	System_ExtrinsicSuccess            []EventSystemExtrinsicSuccess            //nolint:stylecheck,golint
	System_ExtrinsicFailed             []EventSystemExtrinsicFailed             //nolint:stylecheck,golint
	System_CodeUpdated                 []EventSystemCodeUpdated                 //nolint:stylecheck,golint
	System_NewAccount                  []EventSystemNewAccount                  //nolint:stylecheck,golint
	System_KilledAccount               []EventSystemKilledAccount               //nolint:stylecheck,golint
	Assets_Issued                      []EventAssetIssued                       //nolint:stylecheck,golint
	Assets_Transferred                 []EventAssetTransferred                  //nolint:stylecheck,golint
	Assets_Destroyed                   []EventAssetDestroyed                    //nolint:stylecheck,golint
	Democracy_Proposed                 []EventDemocracyProposed                 //nolint:stylecheck,golint
	Democracy_Tabled                   []EventDemocracyTabled                   //nolint:stylecheck,golint
	Democracy_ExternalTabled           []EventDemocracyExternalTabled           //nolint:stylecheck,golint
	Democracy_Started                  []EventDemocracyStarted                  //nolint:stylecheck,golint
	Democracy_Passed                   []EventDemocracyPassed                   //nolint:stylecheck,golint
	Democracy_NotPassed                []EventDemocracyNotPassed                //nolint:stylecheck,golint
	Democracy_Cancelled                []EventDemocracyCancelled                //nolint:stylecheck,golint
	Democracy_Executed                 []EventDemocracyExecuted                 //nolint:stylecheck,golint
	Democracy_Delegated                []EventDemocracyDelegated                //nolint:stylecheck,golint
	Democracy_Undelegated              []EventDemocracyUndelegated              //nolint:stylecheck,golint
	Democracy_Vetoed                   []EventDemocracyVetoed                   //nolint:stylecheck,golint
	Democracy_PreimageNoted            []EventDemocracyPreimageNoted            //nolint:stylecheck,golint
	Democracy_PreimageUsed             []EventDemocracyPreimageUsed             //nolint:stylecheck,golint
	Democracy_PreimageInvalid          []EventDemocracyPreimageInvalid          //nolint:stylecheck,golint
	Democracy_PreimageMissing          []EventDemocracyPreimageMissing          //nolint:stylecheck,golint
	Democracy_PreimageReaped           []EventDemocracyPreimageReaped           //nolint:stylecheck,golint
	Democracy_Unlocked                 []EventDemocracyUnlocked                 //nolint:stylecheck,golint
	Council_Proposed                   []EventCollectiveProposed                //nolint:stylecheck,golint
	Council_Voted                      []EventCollectiveProposed                //nolint:stylecheck,golint
	Council_Approved                   []EventCollectiveApproved                //nolint:stylecheck,golint
	Council_Disapproved                []EventCollectiveDisapproved             //nolint:stylecheck,golint
	Council_Executed                   []EventCollectiveExecuted                //nolint:stylecheck,golint
	Council_MemberExecuted             []EventCollectiveMemberExecuted          //nolint:stylecheck,golint
	Council_Closed                     []EventCollectiveClosed                  //nolint:stylecheck,golint
	TechnicalCommittee_Proposed        []EventTechnicalCommitteeProposed        //nolint:stylecheck,golint
	TechnicalCommittee_Voted           []EventTechnicalCommitteeVoted           //nolint:stylecheck,golint
	TechnicalCommittee_Approved        []EventTechnicalCommitteeApproved        //nolint:stylecheck,golint
	TechnicalCommittee_Disapproved     []EventTechnicalCommitteeDisapproved     //nolint:stylecheck,golint
	TechnicalCommittee_Executed        []EventTechnicalCommitteeExecuted        //nolint:stylecheck,golint
	TechnicalCommittee_MemberExecuted  []EventTechnicalCommitteeMemberExecuted  //nolint:stylecheck,golint
	TechnicalCommittee_Closed          []EventTechnicalCommitteeClosed          //nolint:stylecheck,golint
	Elections_NewTerm                  []EventElectionsNewTerm                  //nolint:stylecheck,golint
	Elections_EmptyTerm                []EventElectionsEmptyTerm                //nolint:stylecheck,golint
	Elections_MemberKicked             []EventElectionsMemberKicked             //nolint:stylecheck,golint
	Elections_MemberRenounced          []EventElectionsMemberRenounced          //nolint:stylecheck,golint
	Elections_VoterReported            []EventElectionsVoterReported            //nolint:stylecheck,golint
	Identity_IdentitySet               []EventIdentitySet                       //nolint:stylecheck,golint
	Identity_IdentityCleared           []EventIdentityCleared                   //nolint:stylecheck,golint
	Identity_IdentityKilled            []EventIdentityKilled                    //nolint:stylecheck,golint
	Identity_JudgementRequested        []EventIdentityJudgementRequested        //nolint:stylecheck,golint
	Identity_JudgementUnrequested      []EventIdentityJudgementUnrequested      //nolint:stylecheck,golint
	Identity_JudgementGiven            []EventIdentityJudgementGiven            //nolint:stylecheck,golint
	Identity_RegistrarAdded            []EventIdentityRegistrarAdded            //nolint:stylecheck,golint
	Identity_SubIdentityAdded          []EventIdentitySubIdentityAdded          //nolint:stylecheck,golint
	Identity_SubIdentityRemoved        []EventIdentitySubIdentityRemoved        //nolint:stylecheck,golint
	Identity_SubIdentityRevoked        []EventIdentitySubIdentityRevoked        //nolint:stylecheck,golint
	Society_Founded                    []EventSocietyFounded                    //nolint:stylecheck,golint
	Society_Bid                        []EventSocietyBid                        //nolint:stylecheck,golint
	Society_Vouch                      []EventSocietyVouch                      //nolint:stylecheck,golint
	Society_AutoUnbid                  []EventSocietyAutoUnbid                  //nolint:stylecheck,golint
	Society_Unbid                      []EventSocietyUnbid                      //nolint:stylecheck,golint
	Society_Unvouch                    []EventSocietyUnvouch                    //nolint:stylecheck,golint
	Society_Inducted                   []EventSocietyInducted                   //nolint:stylecheck,golint
	Society_SuspendedMemberJudgement   []EventSocietySuspendedMemberJudgement   //nolint:stylecheck,golint
	Society_CandidateSuspended         []EventSocietyCandidateSuspended         //nolint:stylecheck,golint
	Society_MemberSuspended            []EventSocietyMemberSuspended            //nolint:stylecheck,golint
	Society_Challenged                 []EventSocietyChallenged                 //nolint:stylecheck,golint
	Society_Vote                       []EventSocietyVote                       //nolint:stylecheck,golint
	Society_DefenderVote               []EventSocietyDefenderVote               //nolint:stylecheck,golint
	Society_NewMaxMembers              []EventSocietyNewMaxMembers              //nolint:stylecheck,golint
	Society_Unfounded                  []EventSocietyUnfounded                  //nolint:stylecheck,golint
	Society_Deposit                    []EventSocietyDeposit                    //nolint:stylecheck,golint
	Recovery_RecoveryCreated           []EventRecoveryCreated                   //nolint:stylecheck,golint
	Recovery_RecoveryInitiated         []EventRecoveryInitiated                 //nolint:stylecheck,golint
	Recovery_RecoveryVouched           []EventRecoveryVouched                   //nolint:stylecheck,golint
	Recovery_RecoveryClosed            []EventRecoveryClosed                    //nolint:stylecheck,golint
	Recovery_AccountRecovered          []EventRecoveryAccountRecovered          //nolint:stylecheck,golint
	Recovery_RecoveryRemoved           []EventRecoveryRemoved                   //nolint:stylecheck,golint
	Vesting_VestingUpdated             []EventVestingVestingUpdated             //nolint:stylecheck,golint
	Vesting_VestingCompleted           []EventVestingVestingCompleted           //nolint:stylecheck,golint
	Scheduler_Scheduled                []EventSchedulerScheduled                //nolint:stylecheck,golint
	Scheduler_Canceled                 []EventSchedulerCanceled                 //nolint:stylecheck,golint
	Scheduler_Dispatched               []EventSchedulerDispatched               //nolint:stylecheck,golint
	Proxy_ProxyExecuted                []EventProxyProxyExecuted                //nolint:stylecheck,golint
	Proxy_AnonymousCreated             []EventProxyAnonymousCreated             //nolint:stylecheck,golint
	Sudo_Sudid                         []EventSudoSudid                         //nolint:stylecheck,golint
	Sudo_KeyChanged                    []EventSudoKeyChanged                    //nolint:stylecheck,golint
	Sudo_SudoAsDone                    []EventSudoAsDone                        //nolint:stylecheck,golint
	Treasury_Proposed                  []EventTreasuryProposed                  //nolint:stylecheck,golint
	Treasury_Spending                  []EventTreasurySpending                  //nolint:stylecheck,golint
	Treasury_Awarded                   []EventTreasuryAwarded                   //nolint:stylecheck,golint
	Treasury_Rejected                  []EventTreasuryRejected                  //nolint:stylecheck,golint
	Treasury_Burnt                     []EventTreasuryBurnt                     //nolint:stylecheck,golint
	Treasury_Rollover                  []EventTreasuryRollover                  //nolint:stylecheck,golint
	Treasury_Deposit                   []EventTreasuryDeposit                   //nolint:stylecheck,golint
	Treasury_NewTip                    []EventTreasuryNewTip                    //nolint:stylecheck,golint
	Treasury_TipClosing                []EventTreasuryTipClosing                //nolint:stylecheck,golint
	Treasury_TipClosed                 []EventTreasuryTipClosed                 //nolint:stylecheck,golint
	Treasury_TipRetracted              []EventTreasuryTipRetracted              //nolint:stylecheck,golint
	Contracts_Instantiated             []EventContractsInstantiated             //nolint:stylecheck,golint
	Contracts_Evicted                  []EventContractsEvicted                  //nolint:stylecheck,golint
	Contracts_Restored                 []EventContractsRestored                 //nolint:stylecheck,golint
	Contracts_CodeStored               []EventContractsCodeStored               //nolint:stylecheck,golint
	Contracts_ScheduleUpdated          []EventContractsScheduleUpdated          //nolint:stylecheck,golint
	Contracts_ContractExecution        []EventContractsContractExecution        //nolint:stylecheck,golint
	Utility_BatchInterrupted           []EventUtilityBatchInterrupted           //nolint:stylecheck,golint
	Utility_BatchCompleted             []EventUtilityBatchCompleted             //nolint:stylecheck,golint
	Multisig_New                       []EventMultisigNewMultisig               //nolint:stylecheck,golint
	Multisig_Approval                  []EventMultisigApproval                  //nolint:stylecheck,golint
	Multisig_Executed                  []EventMultisigExecuted                  //nolint:stylecheck,golint
	Multisig_Cancelled                 []EventMultisigCancelled                 //nolint:stylecheck,golint
}

// DecodeEventRecords decodes the events records from an EventRecordRaw into a target t using the given Metadata m
// If this method returns an error like `unable to decode Phase for event #x: EOF`, it is likely that you have defined
// a custom event record with a wrong type. For example your custom event record has a field with a length prefixed
// type, such as types.Bytes, where your event in reallity contains a fixed width type, such as a types.U32.
func (e EventRecordsRaw) DecodeEventRecords(m *Metadata, t interface{}) error {
	log.Debug(fmt.Sprintf("will decode event records from raw hex: %#x", e))

	// ensure t is a pointer
	ttyp := reflect.TypeOf(t)
	if ttyp.Kind() != reflect.Ptr {
		return errors.New("target must be a pointer, but is " + fmt.Sprint(ttyp))
	}
	// ensure t is not a nil pointer
	tval := reflect.ValueOf(t)
	if tval.IsNil() {
		return errors.New("target is a nil pointer")
	}
	val := tval.Elem()
	typ := val.Type()
	// ensure val can be set
	if !val.CanSet() {
		return fmt.Errorf("unsettable value %v", typ)
	}
	// ensure val points to a struct
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("target must point to a struct, but is " + fmt.Sprint(typ))
	}

	decoder := scale.NewDecoder(bytes.NewReader(e))

	// determine number of events
	n, err := decoder.DecodeUintCompact()
	if err != nil {
		return err
	}

	log.Debug(fmt.Sprintf("found %v events", n))

	// iterate over events
	for i := uint64(0); i < n.Uint64(); i++ {
		log.Debug(fmt.Sprintf("decoding event #%v", i))

		// decode Phase
		phase := Phase{}
		err := decoder.Decode(&phase)
		if err != nil {
			return fmt.Errorf("unable to decode Phase for event #%v: %v", i, err)
		}

		// decode EventID
		id := EventID{}
		err = decoder.Decode(&id)
		if err != nil {
			return fmt.Errorf("unable to decode EventID for event #%v: %v", i, err)
		}

		log.Debug(fmt.Sprintf("event #%v has EventID %v", i, id))

		// ask metadata for method & event name for event
		moduleName, eventName, err := m.FindEventNamesForEventID(id)
		// moduleName, eventName, err := "System", "ExtrinsicSuccess", nil
		if err != nil {
			return fmt.Errorf("unable to find event with EventID %v in metadata for event #%v: %s", id, i, err)
		}

		log.Debug(fmt.Sprintf("event #%v is in module %v with event name %v", i, moduleName, eventName))

		// check whether name for eventID exists in t
		field := val.FieldByName(fmt.Sprintf("%v_%v", moduleName, eventName))
		if !field.IsValid() {
			return fmt.Errorf("unable to find field %v_%v for event #%v with EventID %v", moduleName, eventName, i, id)
		}

		// create a pointer to with the correct type that will hold the decoded event
		holder := reflect.New(field.Type().Elem())

		// ensure first field is for Phase, last field is for Topics
		numFields := holder.Elem().NumField()
		if numFields < 2 {
			return fmt.Errorf("expected event #%v with EventID %v, field %v_%v to have at least 2 fields "+
				"(for Phase and Topics), but has %v fields", i, id, moduleName, eventName, numFields)
		}
		phaseField := holder.Elem().FieldByIndex([]int{0})
		if phaseField.Type() != reflect.TypeOf(phase) {
			return fmt.Errorf("expected the first field of event #%v with EventID %v, field %v_%v to be of type "+
				"types.Phase, but got %v", i, id, moduleName, eventName, phaseField.Type())
		}
		topicsField := holder.Elem().FieldByIndex([]int{numFields - 1})
		if topicsField.Type() != reflect.TypeOf([]Hash{}) {
			return fmt.Errorf("expected the last field of event #%v with EventID %v, field %v_%v to be of type "+
				"[]types.Hash for Topics, but got %v", i, id, moduleName, eventName, topicsField.Type())
		}

		// set the phase we decoded earlier
		phaseField.Set(reflect.ValueOf(phase))

		// set the remaining fields
		for j := 1; j < numFields; j++ {
			err = decoder.Decode(holder.Elem().FieldByIndex([]int{j}).Addr().Interface())
			if err != nil {
				return fmt.Errorf("unable to decode field %v event #%v with EventID %v, field %v_%v: %v", j, i, id, moduleName,
					eventName, err)
			}
		}

		// add the decoded event to the slice
		field.Set(reflect.Append(field, holder.Elem()))

		log.Debug(fmt.Sprintf("decoded event #%v", i))
	}
	return nil
}

// Phase is an enum describing the current phase of the event (applying the extrinsic or finalized)
type Phase struct {
	IsApplyExtrinsic bool
	AsApplyExtrinsic uint32
	IsFinalization   bool
}

func (p *Phase) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	if err != nil {
		return err
	}

	if b == 0 {
		p.IsApplyExtrinsic = true
		err = decoder.Decode(&p.AsApplyExtrinsic)
	} else if b == 1 {
		p.IsFinalization = true
	}

	if err != nil {
		return err
	}

	return nil
}

func (p Phase) Encode(encoder scale.Encoder) error {
	var err1, err2 error
	if p.IsApplyExtrinsic {
		err1 = encoder.PushByte(0)
		err2 = encoder.Encode(p.AsApplyExtrinsic)
	} else if p.IsFinalization {
		err1 = encoder.PushByte(1)
	}

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}

	return nil
}

// DispatchError is an error occurring during extrinsic dispatch
type DispatchError struct {
	HasModule bool
	Module    uint8
	Error     uint8
}

func (d *DispatchError) Decode(decoder scale.Decoder) error {
	b, err := decoder.ReadOneByte()
	if err != nil {
		return err
	}

	// https://github.com/paritytech/substrate/blob/4da29261bfdc13057a425c1721aeb4ec68092d42/primitives/runtime/src/lib.rs
	// Line 391
	// Enum index 3 for Module Error
	if b == 3 {
		d.HasModule = true
		err = decoder.Decode(&d.Module)
	}
	if err != nil {
		return err
	}

	return decoder.Decode(&d.Error)
}

func (d DispatchError) Encode(encoder scale.Encoder) error {
	var err error
	if d.HasModule {
		err = encoder.PushByte(3)
		if err != nil {
			return err
		}
		err = encoder.Encode(d.Module)
	} else {
		err = encoder.PushByte(0)
	}

	if err != nil {
		return err
	}

	return encoder.Encode(&d.Error)
}

type EventID [2]byte
