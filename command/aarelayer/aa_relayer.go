package aarelayer

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/aarelayer/service"
	"github.com/0xPolygon/polygon-edge/command/helper"
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

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.addr,
		addrFlag,
		fmt.Sprintf("%s:%d", helper.AllInterfacesBinding, defaultPort),
		"rest server address [ip:port]",
	)
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

	pool := service.NewAAPool()
	verification := service.NewAAVerification(config, func(a *service.AATransaction) bool {
		return true
	})
	restService := service.NewAARelayerRestServer(pool, state, verification)

	cmd.Printf("Listening on %s...\n", params.addr)

	return restService.ListenAndServe(params.addr)
}
