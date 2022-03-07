package loadbot

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	"github.com/0xPolygon/polygon-edge/types"
	"math/big"
	"strings"
)

var (
	params = &loadbotParams{}
)

var (
	errInvalidMode   = errors.New("invalid loadbot mode")
	errInvalidValues = errors.New("invalid values")
	errContractPath  = errors.New("contract path not specified")
)

const (
	tpsFlag      = "tps"
	modeFlag     = "mode"
	detailedFlag = "detailed"
	chainIDFlag  = "chain-id"
	senderFlag   = "sender"
	receiverFlag = "receiver"
	valueFlag    = "value"
	countFlag    = "count"
	maxConnsFlag = "max-conns"
	gasPriceFlag = "gas-price"
	gasLimitFlag = "gas-limit"
	contractFlag = "contract"
)

type loadbotParams struct {
	tps      uint64
	chainID  uint64
	count    uint64
	maxConns uint64

	contractPath string

	detailed bool

	modeRaw     string
	senderRaw   string
	receiverRaw string
	valueRaw    string
	gasPriceRaw string
	gasLimitRaw string

	mode             Mode
	sender           types.Address
	receiver         types.Address
	value            *big.Int
	gasPrice         *big.Int
	gasLimit         *big.Int
	contractArtifact *generator.ContractArtifact
}

func (p *loadbotParams) validateFlags() error {
	// Validate the correct mode type
	convMode := Mode(strings.ToLower(p.modeRaw))
	if convMode != transfer && convMode != deploy {
		return errInvalidMode
	}

	// Validate the correct mode params
	if convMode == deploy && p.contractPath == "" {
		return errContractPath
	}

	if err := p.initRawParams(); err != nil {
		return errInvalidValues
	}

	return nil
}

func (p *loadbotParams) initRawParams() error {
	if err := p.initGasValues(); err != nil {
		return err
	}

	if err := p.initAddressValues(); err != nil {
		return err
	}

	if err := p.initTxnValue(); err != nil {
		return err
	}

	if err := p.initContract(); err != nil {
		return err
	}

	return nil
}

func (p *loadbotParams) initGasValues() error {
	var parseErr error

	// Parse the gas price
	if p.gasPriceRaw != "" {
		if p.gasPrice, parseErr = types.ParseUint256orHex(&p.gasPriceRaw); parseErr != nil {
			return fmt.Errorf("failed to decode gas price to value: %w", parseErr)
		}
	}

	// Parse the gas limit
	if p.gasLimitRaw != "" {
		if p.gasLimit, parseErr = types.ParseUint256orHex(&p.gasLimitRaw); parseErr != nil {
			return fmt.Errorf("failed to decode gas limit to value: %w", parseErr)
		}
	}

	return nil
}

func (p *loadbotParams) initAddressValues() error {
	if err := p.sender.UnmarshalText([]byte(p.senderRaw)); err != nil {
		return fmt.Errorf("failed to decode sender address: %w", err)
	}

	if err := p.receiver.UnmarshalText([]byte(p.receiverRaw)); err != nil {
		return fmt.Errorf("failed to decode receiver address: %w", err)
	}

	return nil
}

func (p *loadbotParams) initTxnValue() error {
	value, err := types.ParseUint256orHex(&p.valueRaw)
	if err != nil {
		return fmt.Errorf("failed to decode to value: %w", err)
	}

	p.value = value

	return nil
}

func (p *loadbotParams) initContract() error {
	var readErr error

	p.contractArtifact = &generator.ContractArtifact{
		Bytecode: generator.DefaultContractBytecode,
	}

	if p.contractPath != "" {
		if p.contractArtifact, readErr = generator.ReadContractArtifact(
			p.contractPath,
		); readErr != nil {
			return fmt.Errorf(
				"failed to read contract bytecode: %w",
				readErr,
			)
		}
	}

	return nil
}

func (p *loadbotParams) getRequiredFlags() []string {
	return []string{
		senderFlag,
	}
}

func (p *loadbotParams) generateConfig(
	jsonRPCAddress string,
	grpcAddress string,
) *Configuration {
	return &Configuration{
		TPS:              p.tps,
		Sender:           p.sender,
		Receiver:         p.receiver,
		Count:            p.count,
		Value:            p.value,
		JSONRPC:          jsonRPCAddress,
		GRPC:             grpcAddress,
		MaxConns:         int(p.maxConns),
		GeneratorMode:    p.mode,
		ChainID:          p.chainID,
		GasPrice:         p.gasPrice,
		GasLimit:         p.gasLimit,
		ContractArtifact: p.contractArtifact,
	}
}
