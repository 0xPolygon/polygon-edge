package init

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
)

const (
	// maxInitNum is the maximum value for "num" flag
	maxInitNum = 30
)

var (
	errInvalidNum = fmt.Errorf("num flag value should be between 1 and %d", maxInitNum)

	basicParams initParams
	initNumber  int
)

func GetCommand() *cobra.Command {
	secretsInitCmd := &cobra.Command{
		Use: "init",
		Short: "Initializes private keys for the Polygon Edge (Validator + Networking) " +
			"to the specified Secrets Manager",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(secretsInitCmd)

	return secretsInitCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&basicParams.dataDir,
		dataDirFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&basicParams.configPath,
		configFlag,
		"",
		"the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)

	cmd.Flags().IntVar(
		&initNumber,
		numFlag,
		1,
		"the flag indicating how many secrets should be created, only for the local FS",
	)

	// Don't accept data-dir and config flags because they are related to different secrets managers.
	// data-dir is about the local FS as secrets storage, config is about remote secrets manager.
	cmd.MarkFlagsMutuallyExclusive(dataDirFlag, configFlag)

	// num flag should be used with data-dir flag only so it should not be used with config flag.
	cmd.MarkFlagsMutuallyExclusive(numFlag, configFlag)

	cmd.Flags().BoolVar(
		&basicParams.generatesECDSA,
		ecdsaFlag,
		true,
		"the flag indicating whether new ECDSA key is created",
	)

	cmd.Flags().BoolVar(
		&basicParams.generatesNetwork,
		networkFlag,
		true,
		"the flag indicating whether new Network key is created",
	)

	cmd.Flags().BoolVar(
		&basicParams.generatesBLS,
		blsFlag,
		true,
		"the flag indicating whether new BLS key is created",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	if initNumber < 1 || initNumber > maxInitNum {
		return errInvalidNum
	}

	return basicParams.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	paramsList := newParamsList(basicParams, initNumber)
	results := make(Results, len(paramsList))

	for i, params := range paramsList {
		if err := params.initSecrets(); err != nil {
			outputter.SetError(err)

			return
		}

		res, err := params.getResult()
		if err != nil {
			outputter.SetError(err)

			return
		}

		results[i] = res
	}

	outputter.SetCommandResult(results)
}

// newParamsList creates a list of initParams with num elements.
// This function basically copies the given initParams but updating dataDir by applying an index.
func newParamsList(params initParams, num int) []initParams {
	if num == 1 {
		return []initParams{params}
	}

	paramsList := make([]initParams, num)
	for i := 1; i <= num; i++ {
		paramsList[i-1] = initParams{
			dataDir:          fmt.Sprintf("%s%d", params.dataDir, i),
			generatesECDSA:   params.generatesECDSA,
			generatesBLS:     params.generatesBLS,
			generatesNetwork: params.generatesNetwork,
		}
	}

	return paramsList
}
