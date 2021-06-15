package system

import "fmt"

// dummyHandler is just an example handler for system contract
type dummyHandler struct {
	s *System
}

// gas returns the fixed gas price of the operation
func (d dummyHandler) gas(_ []byte) uint64 {
	fmt.Print("\n\n[Dummy Handler] Gas calculation called\n\n")

	return 40000
}

// run executes the system contract method
func (d dummyHandler) run(input []byte) ([]byte, error) {
	fmt.Printf("\n\n[Dummy Handler] Called with input %v\n\n", input)

	return input, nil
}
