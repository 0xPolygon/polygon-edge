package command

import (
	"fmt"
	"os"
)

type CLIOutput struct {
	commonOutputFormatter
}

func newCLIOutput() *CLIOutput {
	return &CLIOutput{}
}

func (cli *CLIOutput) WriteOutput() {
	if cli.errorOutput != nil {
		_, _ = fmt.Fprintln(os.Stderr, cli.getErrorOutput())

		return
	}

	_, _ = fmt.Fprintln(os.Stdout, cli.getCommandOutput())
}

func (cli *CLIOutput) getErrorOutput() string {
	return cli.errorOutput.Error()
}

func (cli *CLIOutput) getCommandOutput() string {
	return cli.commandOutput.GetOutput()
}
