package compiler

import "fmt"

type factory func(path string) Compiler

var compilers = map[string]factory{
	"solidity": NewSolidityCompiler,
	"vyper":    NewVyperCompiler,
}

// Compiler is an Ethereum compiler
type Compiler interface {
	// Compile compiles a file
	Compile(files ...string) (map[string]*Artifact, error)
}

// NewCompiler instantiates a new compiler
func NewCompiler(name string, path string) (Compiler, error) {
	factory, ok := compilers[name]
	if !ok {
		return nil, fmt.Errorf("unknown compiler '%s'", name)
	}
	return factory(path), nil
}

// Artifact is a contract output from the compiler
type Artifact struct {
	Abi        string
	Bin        string
	BinRuntime string
}
