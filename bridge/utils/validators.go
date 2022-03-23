package utils

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

type ValidatorSet interface {
	SetValidators([]types.Address, uint64)
	Validators() []types.Address
	Threshold() uint64
	IsValidator(types.Address) bool
	Subscribe() UpdateValidatorsSubscription
	Unsubscribe(UpdateValidatorsSubscription)
}

type UpdateValidatorsSubscription interface {
	Subscribe(func(UpdateValidatorsEvent))
	push(UpdateValidatorsEvent)
}

type updateValidatorsSubscription struct {
	ch chan UpdateValidatorsEvent
}

func (s *updateValidatorsSubscription) push(event UpdateValidatorsEvent) {
	s.ch <- event
}

func (s *updateValidatorsSubscription) Subscribe(cb func(UpdateValidatorsEvent)) {
	go func() {
		for ev := range s.ch {
			cb(ev)
		}
	}()
}

func newUpdateValidatorsSubscription() UpdateValidatorsSubscription {
	return &updateValidatorsSubscription{
		ch: make(chan UpdateValidatorsEvent, 1),
	}
}

type UpdateValidatorsEvent struct {
	ValidatorAddition []types.Address
	ValidatorDeletion []types.Address
	OldThreshold      uint64
	NewThreshold      uint64
}

type validatorSet struct {
	validatorsLock sync.RWMutex
	validators     []types.Address
	isValidator    map[types.Address]bool
	threshold      uint64

	updateSubsLock sync.RWMutex
	updateSubs     []UpdateValidatorsSubscription
}

func NewValidatorSet(validators []types.Address, threshold uint64) ValidatorSet {
	vs := &validatorSet{}
	vs.SetValidators(validators, threshold)

	return vs
}

func (v *validatorSet) SetValidators(validators []types.Address, threshold uint64) {
	v.validatorsLock.Lock()

	oldValidators := v.validators
	oldThreshold := v.threshold

	v.validators = validators
	v.threshold = threshold
	v.isValidator = make(map[types.Address]bool, len(validators))

	for _, addr := range v.validators {
		v.isValidator[addr] = true
	}

	v.validatorsLock.Unlock()

	v.notifyUpdateValidatorsEvent(oldValidators, validators, oldThreshold, threshold)
}

func (v *validatorSet) Validators() []types.Address {
	v.validatorsLock.RLock()
	defer v.validatorsLock.RUnlock()

	return v.validators
}

func (v *validatorSet) Threshold() uint64 {
	v.validatorsLock.RLock()
	defer v.validatorsLock.RUnlock()

	return v.threshold
}

func (v *validatorSet) IsValidator(addr types.Address) bool {
	v.validatorsLock.RLock()
	defer v.validatorsLock.RUnlock()

	return v.isValidator[addr]
}

func (v *validatorSet) Subscribe() UpdateValidatorsSubscription {
	v.updateSubsLock.Lock()
	defer v.updateSubsLock.Unlock()

	sub := newUpdateValidatorsSubscription()
	v.updateSubs = append(v.updateSubs, sub)

	return sub
}

func (v *validatorSet) Unsubscribe(delSub UpdateValidatorsSubscription) {
	v.updateSubsLock.Lock()
	defer v.updateSubsLock.Unlock()

	for i, sub := range v.updateSubs {
		if sub == delSub {
			v.updateSubs = append(v.updateSubs[:i], v.updateSubs[i+1:]...)

			return
		}
	}
}

func (v *validatorSet) notifyUpdateValidatorsEvent(
	oldValidators []types.Address,
	newValidators []types.Address,
	oldThreshold uint64,
	newThreshold uint64,
) {
	v.updateSubsLock.RLock()
	defer v.updateSubsLock.RUnlock()

	event := UpdateValidatorsEvent{
		ValidatorAddition: common.DiffAddresses(newValidators, oldValidators),
		ValidatorDeletion: common.DiffAddresses(oldValidators, newValidators),
		OldThreshold:      oldThreshold,
		NewThreshold:      newThreshold,
	}

	for _, sub := range v.updateSubs {
		sub.push(event)
	}
}
