package command

import (
	"fmt"
	"os"
)

// cliOutput implements OutputFormatter interface by printing the output into std out
type cliOutput struct {
	commonOutputFormatter
}

// newCLIOutput is the constructor of cliOutput
func newCLIOutput() *cliOutput {
	return &cliOutput{}
}

// WriteOutput implements OutputFormatter interface
func (cli *cliOutput) WriteOutput() {
	if cli.errorOutput != nil {
		_, _ = fmt.Fprintln(os.Stderr, cli.getErrorOutput())

		// return proper error exit code for cli error output
		os.Exit(1)
	}

	_, _ = fmt.Fprintln(os.Stdout, cli.getCommandOutput())
}

// WriteCommandResult implements OutputFormatter interface
func (cli *cliOutput) WriteCommandResult(result CommandResult) {
	_, _ = fmt.Fprintln(os.Stdout, result.GetOutput())
}

// WriteOutput implements OutputFormatter plus io.Writer interfaces
func (cli *cliOutput) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

func (cli *cliOutput) getErrorOutput() string {
	if cli.errorOutput == nil {
		return ""
	}

	return cli.errorOutput.Error()
}

func (cli *cliOutput) getCommandOutput() string {
	if cli.commandOutput == nil {
		return ""
	}

	return cli.commandOutput.GetOutput()
}
