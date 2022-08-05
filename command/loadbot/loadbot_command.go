package loadbot

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	loadbotCmd := &cobra.Command{
		Use:     "loadbot",
		Short:   "Runs the loadbot to stress test the network",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(loadbotCmd)
	helper.RegisterJSONRPCFlag(loadbotCmd)

	setFlags(loadbotCmd)
	helper.SetRequiredFlags(loadbotCmd, params.getRequiredFlags())

	return loadbotCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().Uint64Var(
		&params.tps,
		tpsFlag,
		100,
		"number of transactions to send per second.",
	)

	cmd.Flags().Uint64Var(
		&params.chainID,
		chainIDFlag,
		100,
		"the network chain ID.",
	)

	cmd.Flags().Uint64Var(
		&params.count,
		countFlag,
		1000,
		"the number of transactions to sent in total.",
	)

	cmd.Flags().StringVar(
		&params.modeRaw,
		modeFlag,
		string(transfer),
		"the mode of operation [transfer, deploy, erc20, erc721].",
	)

	cmd.Flags().StringVar(
		&params.senderRaw,
		senderFlag,
		"",
		"the account used to send the transactions",
	)

	cmd.Flags().StringVar(
		&params.receiverRaw,
		receiverFlag,
		"",
		"the account used to receive the transactions",
	)

	cmd.Flags().StringVar(
		&params.valueRaw,
		valueFlag,
		"0x100",
		"the value sent in each transaction in wei.",
	)
	// override default value for help output
	cmd.Flag(valueFlag).DefValue = "100"

	cmd.Flags().StringVar(
		&params.gasPriceRaw,
		gasPriceFlag,
		"",
		"the gas price that should be used for the transactions. If omitted, the average gas price is "+
			"fetched from the network",
	)

	cmd.Flags().StringVar(
		&params.gasLimitRaw,
		gasLimitFlag,
		"",
		"the gas limit that should be used for the transactions. If omitted, the gas limit is "+
			"estimated before starting the loadbot",
	)

	cmd.Flags().StringVar(
		&params.contractPath,
		contractFlag,
		"",
		"the path to the contract JSON artifact containing the bytecode. If omitted, a default "+
			"contract is used",
	)

	cmd.Flags().BoolVar(
		&params.detailed,
		detailedFlag,
		false,
		"flag indicating if the error logs should be shown",
	)

	cmd.Flags().Uint64Var(
		&params.maxConns,
		maxConnsFlag,
		1000000,
		"sets the maximum no. of connections allowed per host.",
	)

	cmd.Flags().Uint64Var(
		&params.maxWait,
		maxWaitFlag,
		0,
		"sets the maximum wait time for transactions receipts in minutes.",
	)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	// initialize raw parameters
	if err := params.initRawParams(); err != nil {
		return errInvalidValues
	}

	if _, err := helper.ParseGRPCAddress(
		helper.GetGRPCAddress(cmd),
	); err != nil {
		return err
	}

	if _, err := helper.ParseJSONRPCAddress(
		helper.GetJSONRPCAddress(cmd),
	); err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	config := params.generateConfig(
		helper.GetJSONRPCAddress(cmd),
		helper.GetGRPCAddress(cmd),
	)

	runResults, err := runLoadbot(config, params.detailed)
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(runResults)
}

func runLoadbot(config *Configuration, detailed bool) (*LoadbotResult, error) {
	loadbot := NewLoadbot(config)

	if err := loadbot.Run(); err != nil {
		return nil, fmt.Errorf(
			"an error occurred while running the loadbot: %w",
			err,
		)
	}

	result := newLoadbotResult(
		loadbot.GetMetrics(),
		config.GeneratorMode,
	)

	if detailed {
		result.initDetailedErrors(loadbot.GetGenerator())
	}

	return result, nil
}
