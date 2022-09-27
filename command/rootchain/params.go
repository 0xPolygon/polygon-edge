package rootchain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/umbracle/ethgo/abi"
)

const (
	dirFlag                = "dir"
	rootchainAddrFlag      = "rootchain-addr"
	eventABIFlag           = "event-abi"
	methodABIFlag          = "method-abi"
	localAddrFlag          = "local-addr"
	payloadTypeFlag        = "payload-type"
	blockConfirmationsFlag = "block-confirmations"
	methodNameFlag         = "method-name"
)

var (
	params = &rootchainParams{}
)

const (
	defaultRootchainName = "rootchain.json"
)

var (
	errInvalidLocalAddr   = errors.New("invalid local SC address")
	errInvalidPayloadType = errors.New("invalid payload type")
	errInvalidEventABI    = errors.New("invalid event ABI")
	errInvalidMethodABI   = errors.New("invalid method ABI")
	errInvalidMethodName  = errors.New("invalid method name")
	errUnableToReadConfig = errors.New("unable to read rootchain config")
)

type rootchainParams struct {
	configPath         string
	rootchainAddr      string
	eventABI           string
	methodABI          string
	methodName         string
	localAddr          string
	payloadType        uint64
	blockConfirmations uint64

	config *rootchain.Config
}

func (p *rootchainParams) getRequiredFlags() []string {
	return []string{
		rootchainAddrFlag,
		eventABIFlag,
		methodABIFlag,
		localAddrFlag,
		methodNameFlag,
	}
}

func (p *rootchainParams) validateFlags() error {
	// Check if the payload type is supported
	if err := validatePayloadType(p.payloadType); err != nil {
		return err
	}

	// Check if the event ABI is valid
	if err := p.validateEventABI(); err != nil {
		return err
	}

	// Check if the method ABI is valid
	if err := p.validateMethodABI(); err != nil {
		return err
	}

	// Check if the local address is valid
	return p.validateLocalAddress()
}

func validatePayloadType(payloadType uint64) error {
	if rootchain.PayloadType(payloadType) != rootchain.ValidatorSetPayloadType {
		return errInvalidPayloadType
	}

	return nil
}

func (p *rootchainParams) validateEventABI() error {
	_, err := abi.NewEvent(p.eventABI)

	if err != nil {
		return errInvalidEventABI
	}

	return nil
}

func (p *rootchainParams) validateMethodABI() error {
	// Make sure the ABI is correct
	methodABI, err := abi.NewABI(p.methodABI)
	if err != nil {
		return errInvalidMethodABI
	}

	// Make sure the method is present in the ABI
	if methodABI.GetMethod(p.methodName) == nil {
		return errInvalidMethodName
	}

	return nil
}

func (p *rootchainParams) validateLocalAddress() error {
	if len(p.localAddr) == 0 {
		return errInvalidLocalAddr
	}

	return nil
}

func (p *rootchainParams) loadExistingConfig() error {
	_, err := os.Stat(p.configPath)
	if err != nil && !os.IsNotExist(err) {
		return errors.New("unable to stat rootchain config")
	}

	if !os.IsNotExist(err) {
		// The rootchain config file exists, load it
		cc, err := importFromFile(p.configPath)
		if err != nil {
			return err
		}

		p.config = cc
	}

	return nil
}

func (p *rootchainParams) generateConfig() error {
	var (
		config         *rootchain.Config
		existingEvents = make([]rootchain.ConfigEvent, 0)
	)

	if p.config != nil {
		config = p.config
	} else {
		config = &rootchain.Config{
			RootchainAddresses: make(map[string][]rootchain.ConfigEvent),
		}
	}

	if len(config.RootchainAddresses[p.rootchainAddr]) > 0 {
		existingEvents = config.RootchainAddresses[p.rootchainAddr]
	}

	// Append the event
	existingEvents = append(
		existingEvents,
		rootchain.ConfigEvent{
			EventABI:           p.eventABI,
			MethodABI:          p.methodABI,
			LocalAddress:       p.localAddr,
			PayloadType:        rootchain.PayloadType(p.payloadType),
			BlockConfirmations: p.blockConfirmations,
			MethodName:         p.methodName,
		},
	)

	config.RootchainAddresses[p.rootchainAddr] = existingEvents

	p.config = config

	// Write the config to the disk
	return p.writeConfigToDisk()
}

func (p *rootchainParams) writeConfigToDisk() error {
	if _, err := os.Stat(p.configPath); err == nil {
		// Remove the current rootchain configuration from disk
		if err := os.Remove(p.configPath); err != nil {
			return err
		}
	}

	// Save the new rootchain configuration
	return helper.WriteRootchainConfigToDisk(p.config, p.configPath)
}

func importFromFile(fileName string) (*rootchain.Config, error) {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var config *rootchain.Config

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errUnableToReadConfig
	}

	return config, nil
}

func (p *rootchainParams) getResult() command.CommandResult {
	return &RootchainResult{
		Message: fmt.Sprintf("Rootchain configuration written to %s\n", p.configPath),
	}
}
