package init

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

const (
	// maxInitNum is the maximum value for "num" flag
	maxInitNum = 1000
)

var (
	errInvalidNum = fmt.Errorf("num flag value should be between 1 and %d", maxInitNum)
)

func GetCommand() *cobra.Command {
	var basicParams initParams
	var number int

	secretsInitCmd := &cobra.Command{
		Use: "init",
		Short: "Initializes private keys for the Polygon Edge (Validator + Networking) " +
			"to the specified Secrets Manager",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if number < 1 || number > maxInitNum {
				return errInvalidNum
			}

			return basicParams.validateFlags()
		},
		Run: func(cmd *cobra.Command, _ []string) {
			outputter := command.InitializeOutputter(cmd)
			defer outputter.WriteOutput()

			paramsList := newParamsList(basicParams, number)
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
		},
	}

	setFlags(secretsInitCmd, &basicParams, &number)

	return secretsInitCmd
}

func setFlags(cmd *cobra.Command, params *initParams, num *int) {
	cmd.Flags().StringVar(
		&params.dataDir,
		dataDirFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&params.configPath,
		configFlag,
		"",
		"the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)

	cmd.Flags().IntVar(
		num,
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
		&params.generatesECDSA,
		ecdsaFlag,
		true,
		"the flag indicating whether new ECDSA key is created",
	)

	cmd.Flags().BoolVar(
		&params.generatesNetwork,
		networkFlag,
		true,
		"the flag indicating whether new Network key is created",
	)

	cmd.Flags().BoolVar(
		&params.generatesBLS,
		blsFlag,
		true,
		"the flag indicating whether new BLS key is created",
	)
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
