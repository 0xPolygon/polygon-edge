package testutil

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/umbracle/go-web3/compiler"
	"golang.org/x/crypto/sha3"
)

// Contract is a test contract
type Contract struct {
	events    []*Event
	callbacks []func() string
}

// AddEvent adds a new event to the contract
func (c *Contract) AddEvent(e *Event) {
	if c.events == nil {
		c.events = []*Event{}
	}
	c.addCallback(e.printDecl)
	c.addCallback(e.printSetter)
	c.events = append(c.events, e)
}

// GetEvent returns the event with the given name
func (c *Contract) GetEvent(name string) *Event {
	for _, i := range c.events {
		if i.name == name {
			return i
		}
	}
	return nil
}

// Print prints the contract
func (c *Contract) Print() string {
	str := "pragma solidity ^0.5.5;\n"
	str += "pragma experimental ABIEncoderV2;\n"
	str += "\n"
	str += "contract Sample {\n"

	for _, f := range c.callbacks {
		str += f() + "\n"
	}

	str += "}"
	return str
}

func (c *Contract) addCallback(f func() string) {
	if c.callbacks == nil {
		c.callbacks = []func() string{}
	}
	c.callbacks = append(c.callbacks, f)
}

// Compile compiles the contract
func (c *Contract) Compile() (*compiler.Artifact, error) {
	output, err := compiler.NewSolidityCompiler("solc").(*compiler.Solidity).CompileCode(c.Print())
	if err != nil {
		return nil, err
	}
	solcContract, ok := output["<stdin>:Sample"]
	if !ok {
		return nil, fmt.Errorf("Expected the contract to be called Sample")
	}
	return solcContract, nil
}

// AddConstructor creates a constructor with args that set contract variables
func (c *Contract) AddConstructor(args ...string) {
	// add variables
	c.addCallback(func() string {
		str := ""

		input := []string{}
		body := ""
		for indx, arg := range args {
			str += fmt.Sprintf("%s public val_%d;\n", arg, indx)

			input = append(input, fmt.Sprintf("%s local_%d", arg, indx))
			body += fmt.Sprintf("val_%d = local_%d;\n", indx, indx)
		}

		str += "constructor(" + strings.Join(input, ",") + ") public {\n"
		str += body
		str += "}"
		return str
	})
}

// AddDualCaller adds a call function that returns the same values that takes
func (c *Contract) AddDualCaller(funcName string, args ...string) {
	c.addCallback(func() string {
		var params, rets, body []string
		for indx, i := range args {
			name := "val_" + strconv.Itoa(indx)
			// function params
			params = append(params, i+" "+name)
			// function returns
			rets = append(rets, i)
			// function body
			body = append(body, name)
		}

		str := "function " + funcName + "(" + strings.Join(params, ",") + ") public view returns (" + strings.Join(rets, ",") + ") {\n"
		str += "return (" + strings.Join(body, ",") + ");\n"
		str += "}"
		return str
	})
}

// EmitEvent emits a specific event
func (c *Contract) EmitEvent(funcName string, name string, args ...string) {
	exists := false
	for _, i := range c.events {
		if i.name == name {
			exists = true
		}
	}
	if !exists {
		panic(fmt.Errorf("event %s does not exists", name))
	}
	c.addCallback(func() string {
		str := "function " + funcName + "() public payable {\n"
		str += fmt.Sprintf("emit %s(%s)", name, strings.Join(args, ", ")) + ";\n"
		str += "}"
		return str
	})
}

type eventField struct {
	indexed bool
	typ     string
}

// Event is a test event
type Event struct {
	name   string
	fields []*eventField
}

func (e *Event) printDecl() string {
	args := []string{}
	for indx, i := range e.fields {
		arg := i.typ
		if i.indexed {
			arg += " indexed"
		}
		arg += " val_" + strconv.Itoa(indx)
		args = append(args, arg)
	}
	return fmt.Sprintf("event %s(%s);", e.name, strings.Join(args, ", "))
}

func (e *Event) printSetter() string {
	params := []string{}
	body := []string{}
	for indx, i := range e.fields {
		// function params

		// Some params are in memory
		typ := i.typ
		if typ == "string" || strings.Contains(typ, "[") {
			typ = typ + " memory"
		}
		params = append(params, fmt.Sprintf("%s val_%d", typ, indx))
		// function body
		body = append(body, fmt.Sprintf("val_%d", indx))
	}

	str := "function setter" + e.name + "(" + strings.Join(params, ", ") + ") public payable {\n"
	str += "emit " + e.name + "(" + strings.Join(body, ", ") + ");\n"
	str += "}"
	return str
}

// Add adds a new field to the event
func (e *Event) Add(typStr string, indexed bool) *Event {
	if e.fields == nil {
		e.fields = []*eventField{}
	}
	e.fields = append(e.fields, &eventField{indexed, typStr})
	return e
}

// Sig returns the signature of the event
func (e *Event) Sig() string {
	args := []string{}
	for _, i := range e.fields {
		args = append(args, i.typ)
	}

	signature := e.name + "(" + strings.Join(args, ",") + ")"

	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(signature))
	b := h.Sum(nil)
	return "0x" + hex.EncodeToString(b)
}

// NewEvent creates a new contract event
func NewEvent(name string, args ...interface{}) *Event {
	if len(args)%2 != 0 {
		panic("it should be even")
	}
	e := &Event{name: name}
	for i := 0; i < len(args); i++ {
		e.Add(args[i].(string), args[i+1].(bool))
	}
	return e
}
