package aarelayer

import (
	"github.com/0xPolygon/polygon-edge/command/aarelayer/service"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var params aarelayerParams

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "aarelayer",
		Short:   "Account abstraction relayer",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(cmd)

	return cmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	state, err := service.NewAATxState()
	if err != nil {
		return err
	}

	config, err := service.GetConfig()
	if err != nil {
		return err
	}

	secretsManager, err := polybftsecrets.GetSecretsManager(params.accountDir, params.configPath, true)
	if err != nil {
		return err
	}

	newValidatorAccount, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	pool := service.NewAAPool()
	verification := service.NewAAVerification(
		config,
		types.Address(newValidatorAccount.Ecdsa.Address()),
		params.chainID,
		func(*service.AATransaction) bool { return true })
	restService := service.NewAARelayerRestServer(pool, state, verification)

	cmd.Printf("Listening on %s...\n", params.addr)

	return restService.ListenAndServe(params.addr)
}
