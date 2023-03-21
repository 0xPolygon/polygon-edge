package proto

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/validate"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/assert"
)

func TestRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      validate.Validator
		valid    bool
		errorMsg string
	}{
		{
			name: "AddTxnReq: raw not set",
			req: &AddTxnReq{
				Raw:  nil,
				From: "0x9FC184A287e4BB51Eef4ecA81788eA10EF3f202f",
			},
			valid:    false,
			errorMsg: "invalid AddTxnReq.Raw: value does not match regex pattern",
		},
		{
			name: "AddTxnReq: allow empty address",
			req: &AddTxnReq{
				Raw:  &any.Any{},
				From: "",
			},
			valid: true,
		},
		{
			name: "AddTxnReq: invalid address",
			req: &AddTxnReq{
				Raw:  &any.Any{},
				From: "1234",
			},
			valid:    false,
			errorMsg: "invalid AddTxnReq.From: value does not match regex pattern",
		},
		{
			name: "AddTxnReq: valid address",
			req: &AddTxnReq{
				Raw:  &any.Any{},
				From: "0x9FC184A287e4BB51Eef4ecA81788eA10EF3f202f",
			},
			valid: true,
		},
		{
			name: "SubscribeRequest: type not specified",
			req: &SubscribeRequest{
				Types: []EventType{},
			},
			valid:    false,
			errorMsg: "invalid SubscribeRequest.Types: value must contain at least 1 item(s)",
		},
		{
			name: "SubscribeRequest: duplicated types",
			req: &SubscribeRequest{
				Types: []EventType{EventType_ADDED, EventType_ADDED, EventType_ENQUEUED},
			},
			valid:    false,
			errorMsg: "invalid SubscribeRequest.Types: repeated value must contain unique items",
		},
		{
			name: "SubscribeRequest: undifined type",
			req: &SubscribeRequest{
				Types: []EventType{20},
			},
			valid:    false,
			errorMsg: "invalid SubscribeRequest.Types: value must be one of the defined enum values",
		},
		{
			name: "SubscribeRequest: valid type",
			req: &SubscribeRequest{
				Types: []EventType{EventType_ADDED},
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.valid {
				assert.NoError(t, tt.req.ValidateAll())
			} else {
				assert.Error(t, tt.req.ValidateAll())
			}
		})
	}
}
