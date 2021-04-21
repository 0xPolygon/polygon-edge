package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Vyper is the vyper compiler
type Vyper struct {
	path string
}

// NewVyperCompiler instantiates a new vyper compiler
func NewVyperCompiler(path string) Compiler {
	return &Vyper{path}
}

// Compile implements the compiler interface
func (v *Vyper) Compile(files ...string) (map[string]*Artifact, error) {
	args := []string{
		"-f",
		"combined_json",
	}
	args = append(args, files...)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(v.path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to compile: %s", string(stderr.Bytes()))
	}

	var output map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, err
	}

	artifacts := map[string]*Artifact{}
	for name, o := range output {
		if name == "version" {
			continue
		}

		contract := o.(map[string]interface{})
		abiStr, err := json.Marshal(contract["abi"])
		if err != nil {
			return nil, err
		}

		artifacts[name] = &Artifact{
			Bin:        contract["bytecode"].(string),
			BinRuntime: contract["bytecode_runtime"].(string),
			Abi:        string(abiStr),
		}
	}
	return artifacts, nil
}
