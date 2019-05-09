package api

import (
	"github.com/hashicorp/go-hclog"
)

// An API backend exposes data to external interfaces
type API interface {
	Close() error
}

// Factory is the factory function to create an api backend
type Factory func(logger hclog.Logger, minimal interface{}, config map[string]interface{}) (API, error)
