package validate

import "fmt"

type Validator interface {
	ValidateAll() error
}

func ValidateRequest(req interface{}) error {
	if r, ok := req.(Validator); ok {
		if err := r.ValidateAll(); err != nil {
			return fmt.Errorf("request validation failed: %w", err)
		}
	}

	return nil
}
