package common

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

const (
	receiversFlag = "receivers"
	amountsFlag   = "amounts"
	jsonRPCFlag   = "json-rpc"
	senderKeyFlag = "sender-key"
)

var (
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
)

type BridgeParams struct {
	TxnSenderKey string
	Receivers    []string
	Amounts      []string
}

func (bp *BridgeParams) ValidateFlags() error {
	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}

// GetBridgeParams gathers persistent flags values and creates BridgeParams instance of it
func GetBridgeParams(cmd *cobra.Command) (*BridgeParams, error) {
	receivers, err := cmd.Flags().GetStringSlice(receiversFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", receiversFlag, err)
	}

	amounts, err := cmd.Flags().GetStringSlice(amountsFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", amountsFlag, err)
	}

	txnSenderKey, err := cmd.Flags().GetString(senderKeyFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve value for %s flag: %w", senderKeyFlag, err)
	}

	p := &BridgeParams{
		TxnSenderKey: txnSenderKey,
		Receivers:    receivers,
		Amounts:      amounts,
	}

	return p, nil
}

// RegisterBridgePersistentFlags registers persistent flags which are shared
// between bridge deposit and withdraw commands
func RegisterBridgePersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		senderKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"hex encoded private key of the account which sends rootchain deposit transactions",
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

	cmd.MarkFlagRequired(senderKeyFlag)
	cmd.MarkFlagRequired(receiversFlag)
	cmd.MarkFlagRequired(amountsFlag)
}
