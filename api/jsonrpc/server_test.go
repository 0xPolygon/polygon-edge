package jsonrpc

import (
	"encoding/json"
	"testing"
)

func expectEmptyResult(t *testing.T, data []byte) {
	var i interface{}
	if err := expectJSONResult(data, &i); err != nil {
		t.Fatal(err)
	}
	if i != nil {
		t.Fatal("expected empty result")
	}
}

func expectNonEmptyResult(t *testing.T, data []byte) {
	var i interface{}
	if err := expectJSONResult(data, &i); err != nil {
		t.Fatal(err)
	}
	if i == nil {
		t.Fatal("expected non empty result")
	}
}

func expectJSONResult(data []byte, v interface{}) error {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if err := json.Unmarshal(resp.Result, v); err != nil {
		return err
	}
	return nil
}
