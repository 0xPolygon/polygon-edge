package common

import (
	"errors"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
)

type TokenType int

const (
	ERC20 TokenType = iota
	ERC721
	ERC1155
)

const (
	SenderKeyFlag          = "sender-key"
	ReceiversFlag          = "receivers"
	AmountsFlag            = "amounts"
	TokenIDsFlag           = "token-ids"
	RootTokenFlag          = "root-token"
	RootPredicateFlag      = "root-predicate"
	ChildPredicateFlag     = "child-predicate"
	ChildTokenFlag         = "child-token"
	JSONRPCFlag            = "json-rpc"
	ChildChainMintableFlag = "child-chain-mintable"

	MinterKeyFlag     = "minter-key"
	MinterKeyFlagDesc = "minter key is the account which is able to mint tokens to sender account " +
		"(if provided tokens are minted prior to depositing)"
)

var (
	errInconsistentAmounts  = errors.New("receivers and amounts must be equal length")
	errInconsistentTokenIds = errors.New("receivers and token ids must be equal length")
)

type BridgeParams struct {
	SenderKey          string
	Receivers          []string
	TokenAddr          string
	PredicateAddr      string
	JSONRPCAddr        string
	ChildChainMintable bool
}

// RegisterCommonFlags registers common bridge flags to a given command
func (p *BridgeParams) RegisterCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&p.SenderKey,
		SenderKeyFlag,
		"",
		"hex encoded private key of the account which sends bridge transactions",
	)

	cmd.Flags().StringSliceVar(
		&p.Receivers,
		ReceiversFlag,
		nil,
		"receiving accounts addresses",
	)

	cmd.Flags().StringVar(
		&p.JSONRPCAddr,
		JSONRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the JSON RPC endpoint",
	)

	cmd.Flags().BoolVar(
		&p.ChildChainMintable,
		ChildChainMintableFlag,
		false,
		"flag indicating whether tokens originate from child chain",
	)
}

type ERC20BridgeParams struct {
	*BridgeParams
	Amounts []string
}

func NewERC20BridgeParams() *ERC20BridgeParams {
	return &ERC20BridgeParams{BridgeParams: &BridgeParams{}}
}

func (bp *ERC20BridgeParams) Validate() error {
	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAmounts
	}

	return nil
}

type ERC721BridgeParams struct {
	*BridgeParams
	TokenIDs []string
}

func NewERC721BridgeParams() *ERC721BridgeParams {
	return &ERC721BridgeParams{BridgeParams: &BridgeParams{}}
}

func (bp *ERC721BridgeParams) Validate() error {
	if len(bp.Receivers) != len(bp.TokenIDs) {
		return errInconsistentTokenIds
	}

	return nil
}

type ERC1155BridgeParams struct {
	*BridgeParams
	Amounts  []string
	TokenIDs []string
}

func NewERC1155BridgeParams() *ERC1155BridgeParams {
	return &ERC1155BridgeParams{BridgeParams: &BridgeParams{}}
}

func (bp *ERC1155BridgeParams) Validate() error {
	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAmounts
	}

	if len(bp.Receivers) != len(bp.TokenIDs) {
		return errInconsistentTokenIds
	}

	return nil
}

// ExtractExitEventID tries to extract exit event id from provided receipt
func ExtractExitEventID(receipt *ethgo.Receipt) (*big.Int, error) {
	var exitEvent contractsapi.L2StateSyncedEvent
	for _, log := range receipt.Logs {
		doesMatch, err := exitEvent.ParseLog(log)
		if err != nil {
			return nil, err
		}

		if !doesMatch {
			continue
		}

		return exitEvent.ID, nil
	}

	return nil, errors.New("failed to find exit event log")
}
