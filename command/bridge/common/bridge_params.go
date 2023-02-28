package common

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

const (
	manifestPathFlag = "manifest"
	tokenFlag        = "token"
	receiversFlag    = "receivers"
	amountsFlag      = "amounts"
	jsonRPCFlag      = "json-rpc"
	senderKeyFlag    = "sender-key"
)

var (
	errReceiversMissing     = errors.New("receivers flag value is not provided")
	errAmountsMissing       = errors.New("amount flag value is not provided")
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
	errSenderKeyMissing     = errors.New("sender private key is not provided")
)

type BridgeParams struct {
	ManifestPath   string
	TokenTypeRaw   string
	Receivers      []string
	Amounts        []string
	JSONRPCAddress string
	TxnSenderKey   string
}

func (bp *BridgeParams) ValidateFlags() error {
	if bp.TxnSenderKey == "" {
		return errSenderKeyMissing
	}

	if _, err := os.Stat(bp.ManifestPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("provided manifest path '%s' doesn't exist", bp.ManifestPath)
	}

	if len(bp.Receivers) == 0 {
		return errReceiversMissing
	}

	if len(bp.Amounts) == 0 {
		return errAmountsMissing
	}

	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	if _, exists := LookupTokenType(bp.TokenTypeRaw); !exists {
		return fmt.Errorf("unrecognized token type provided: %s", bp.TokenTypeRaw)
	}

	return nil
}

// GetBridgeParams gathers persistent flags values and creates BridgeParams instance of it
func GetBridgeParams(cmd *cobra.Command) (*BridgeParams, error) {
	manifestPath, err := cmd.Flags().GetString(manifestPathFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", manifestPathFlag, err)
	}

	tokenType, err := cmd.Flags().GetString(tokenFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", tokenFlag, err)
	}

	receivers, err := cmd.Flags().GetStringSlice(receiversFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", receiversFlag, err)
	}

	amounts, err := cmd.Flags().GetStringSlice(amountsFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", amountsFlag, err)
	}

	jsonRPCAddress, err := cmd.Flags().GetString(jsonRPCFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", jsonRPCFlag, err)
	}

	txnSenderKey, err := cmd.Flags().GetString(senderKeyFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", senderKeyFlag, err)
	}

	p := &BridgeParams{
		ManifestPath:   manifestPath,
		TokenTypeRaw:   tokenType,
		Receivers:      receivers,
		Amounts:        amounts,
		JSONRPCAddress: jsonRPCAddress,
		TxnSenderKey:   txnSenderKey,
	}

	return p, nil
}

// RegisterBridgePersistentFlags registers persistent flags which are shared
// between bridge deposit and withdraw commands
func RegisterBridgePersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		manifestPathFlag,
		"./manifest.json",
		"the manifest file path, which contains genesis metadata",
	)

	cmd.PersistentFlags().String(
		tokenFlag,
		"erc20",
		"token type which is being deposited",
	)

	cmd.PersistentFlags().StringSlice(
		receiversFlag,
		nil,
		"receiving accounts addresses on child chain",
	)

	cmd.PersistentFlags().StringSlice(
		amountsFlag,
		nil,
		"amounts to send to child chain receiving accounts",
	)

	cmd.PersistentFlags().String(
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
	)

	cmd.PersistentFlags().String(
		senderKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"hex encoded private key of the account which sends rootchain deposit transactions",
	)
}

type TokenType int

const (
	ERC20 TokenType = iota
	ERC721
	ERC1155
)

var tokenTypesMap = map[string]TokenType{
	"erc20":   ERC20,
	"erc721":  ERC721,
	"erc1155": ERC1155,
}

// LookupTokenType looks up for provided token type string and returns resolved enum value if found
func LookupTokenType(tokenTypeRaw string) (TokenType, bool) {
	tokenType, ok := tokenTypesMap[strings.ToLower(tokenTypeRaw)]

	return tokenType, ok
}
